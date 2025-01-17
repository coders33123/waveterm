// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshremote

import (
	"archive/tar"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/wavetermdev/waveterm/pkg/util/fileutil"
	"github.com/wavetermdev/waveterm/pkg/util/iochan"
	"github.com/wavetermdev/waveterm/pkg/util/utilfn"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
)

type ServerImpl struct {
	LogWriter io.Writer
}

func (*ServerImpl) WshServerImpl() {}

func (impl *ServerImpl) Log(format string, args ...interface{}) {
	if impl.LogWriter != nil {
		fmt.Fprintf(impl.LogWriter, format, args...)
	} else {
		log.Printf(format, args...)
	}
}

func (impl *ServerImpl) MessageCommand(ctx context.Context, data wshrpc.CommandMessageData) error {
	impl.Log("[message] %q\n", data.Message)
	return nil
}

type ByteRangeType struct {
	All   bool
	Start int64
	End   int64
}

func parseByteRange(rangeStr string) (ByteRangeType, error) {
	if rangeStr == "" {
		return ByteRangeType{All: true}, nil
	}
	var start, end int64
	_, err := fmt.Sscanf(rangeStr, "%d-%d", &start, &end)
	if err != nil {
		return ByteRangeType{}, errors.New("invalid byte range")
	}
	if start < 0 || end < 0 || start > end {
		return ByteRangeType{}, errors.New("invalid byte range")
	}
	return ByteRangeType{Start: start, End: end}, nil
}

func (impl *ServerImpl) remoteStreamFileDir(ctx context.Context, path string, byteRange ByteRangeType, dataCallback func(fileInfo []*wshrpc.FileInfo, data []byte, byteRange ByteRangeType)) error {
	innerFilesEntries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("cannot open dir %q: %w", path, err)
	}
	if byteRange.All {
		if len(innerFilesEntries) > wshrpc.MaxDirSize {
			innerFilesEntries = innerFilesEntries[:wshrpc.MaxDirSize]
		}
	} else {
		if byteRange.Start >= int64(len(innerFilesEntries)) {
			return nil
		}
		realEnd := byteRange.End
		if realEnd > int64(len(innerFilesEntries)) {
			realEnd = int64(len(innerFilesEntries))
		}
		innerFilesEntries = innerFilesEntries[byteRange.Start:realEnd]
	}
	var fileInfoArr []*wshrpc.FileInfo
	parent := filepath.Dir(path)
	parentFileInfo, err := impl.fileInfoInternal(parent, false)
	if err == nil && parent != path {
		parentFileInfo.Name = ".."
		parentFileInfo.Size = -1
		fileInfoArr = append(fileInfoArr, parentFileInfo)
	}
	for _, innerFileEntry := range innerFilesEntries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		innerFileInfoInt, err := innerFileEntry.Info()
		if err != nil {
			continue
		}
		innerFileInfo := statToFileInfo(filepath.Join(path, innerFileInfoInt.Name()), innerFileInfoInt, false)
		fileInfoArr = append(fileInfoArr, innerFileInfo)
		if len(fileInfoArr) >= wshrpc.DirChunkSize {
			dataCallback(fileInfoArr, nil, byteRange)
			fileInfoArr = nil
		}
	}
	if len(fileInfoArr) > 0 {
		dataCallback(fileInfoArr, nil, byteRange)
	}
	return nil
}

func (impl *ServerImpl) remoteStreamFileRegular(ctx context.Context, path string, byteRange ByteRangeType, dataCallback func(fileInfo []*wshrpc.FileInfo, data []byte, byteRange ByteRangeType)) error {
	fd, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open file %q: %w", path, err)
	}
	defer fd.Close()
	var filePos int64
	if !byteRange.All && byteRange.Start > 0 {
		_, err := fd.Seek(byteRange.Start, io.SeekStart)
		if err != nil {
			return fmt.Errorf("seeking file %q: %w", path, err)
		}
		filePos = byteRange.Start
	}
	buf := make([]byte, wshrpc.FileChunkSize)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, err := fd.Read(buf)
		if n > 0 {
			if !byteRange.All && filePos+int64(n) > byteRange.End {
				n = int(byteRange.End - filePos)
			}
			filePos += int64(n)
			dataCallback(nil, buf[:n], byteRange)
		}
		if !byteRange.All && filePos >= byteRange.End {
			break
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("reading file %q: %w", path, err)
		}
	}
	return nil
}

func (impl *ServerImpl) remoteStreamFileInternal(ctx context.Context, data wshrpc.CommandRemoteStreamFileData, dataCallback func(fileInfo []*wshrpc.FileInfo, data []byte, byteRange ByteRangeType)) error {
	byteRange, err := parseByteRange(data.ByteRange)
	if err != nil {
		return err
	}
	path, err := wavebase.ExpandHomeDir(data.Path)
	if err != nil {
		return err
	}
	finfo, err := impl.fileInfoInternal(path, true)
	if err != nil {
		return fmt.Errorf("cannot stat file %q: %w", path, err)
	}
	dataCallback([]*wshrpc.FileInfo{finfo}, nil, byteRange)
	if finfo.NotFound {
		return nil
	}
	if finfo.Size > wshrpc.MaxFileSize {
		return fmt.Errorf("file %q is too large to read, use /wave/stream-file", path)
	}
	if finfo.IsDir {
		return impl.remoteStreamFileDir(ctx, path, byteRange, dataCallback)
	} else {
		return impl.remoteStreamFileRegular(ctx, path, byteRange, dataCallback)
	}
}

func (impl *ServerImpl) RemoteStreamFileCommand(ctx context.Context, data wshrpc.CommandRemoteStreamFileData) chan wshrpc.RespOrErrorUnion[wshrpc.FileData] {
	ch := make(chan wshrpc.RespOrErrorUnion[wshrpc.FileData], 16)
	go func() {
		defer close(ch)
		err := impl.remoteStreamFileInternal(ctx, data, func(fileInfo []*wshrpc.FileInfo, data []byte, byteRange ByteRangeType) {
			resp := wshrpc.FileData{}
			fileInfoLen := len(fileInfo)
			if fileInfoLen > 1 {
				resp.Info = fileInfo[0]
				resp.Entries = fileInfo
			} else if fileInfoLen == 1 {
				resp.Info = fileInfo[0]
			}
			if len(data) > 0 {
				resp.Data64 = base64.StdEncoding.EncodeToString(data)
				resp.At = &wshrpc.FileDataAt{Offset: byteRange.Start, Size: int64(len(data))}
			}
			log.Printf("callback -- sending response %d\n", len(resp.Data64))
			ch <- wshrpc.RespOrErrorUnion[wshrpc.FileData]{Response: resp}
		})
		if err != nil {
			ch <- wshutil.RespErr[wshrpc.FileData](err)
		}
	}()
	return ch
}

func (impl *ServerImpl) RemoteTarStreamCommand(ctx context.Context, data wshrpc.CommandRemoteStreamTarData) <-chan wshrpc.RespOrErrorUnion[[]byte] {
	path := data.Path
	opts := data.Opts
	log.Printf("RemoteTarStreamCommand: path=%s\n", path)
	path, err := wavebase.ExpandHomeDir(path)
	if err != nil {
		return wshutil.SendErrCh[[]byte](fmt.Errorf("cannot expand path %q: %w", path, err))
	}
	cleanedPath := filepath.Clean(wavebase.ExpandHomeDirSafe(path))
	finfo, err := os.Stat(cleanedPath)
	if err != nil {
		return wshutil.SendErrCh[[]byte](fmt.Errorf("cannot stat file %q: %w", path, err))
	}
	pipeReader, pipeWriter := io.Pipe()
	tarWriter := tar.NewWriter(pipeWriter)
	iochanCtx, cancel := context.WithCancel(ctx)
	rtn := iochan.ReaderChan(iochanCtx, pipeReader, wshrpc.FileChunkSize, func() {
		pipeReader.Close()
		pipeWriter.Close()
		tarWriter.Close()
	})
	go func() {
		defer cancel()
		if finfo.IsDir() {
			log.Printf("creating tar stream for directory %q\n", path)
			if opts != nil && opts.Recursive {
				log.Printf("creating tar stream for directory %q recursively\n", path)
				err := tarWriter.AddFS(os.DirFS(path))
				if err != nil {
					rtn <- wshutil.RespErr[[]byte](fmt.Errorf("cannot create tar stream for %q: %w", path, err))
					return
				}
				log.Printf("added directory %q to tar stream\n", path)
				log.Printf("returning tar stream\n")
			} else {
				rtn <- wshutil.RespErr[[]byte](fmt.Errorf("cannot create tar stream for %q: %w", path, errors.New("directory copy requires recursive option")))
			}
		} else {
			log.Printf("creating tar stream for file %q\n", path)
			header, err := tar.FileInfoHeader(finfo, "")
			if err != nil {
				rtn <- wshutil.RespErr[[]byte](fmt.Errorf("cannot create tar header for %q: %w", path, err))
				return
			}
			log.Printf("created tar header for file %q\n", path)
			err = tarWriter.WriteHeader(header)
			if err != nil {
				rtn <- wshutil.RespErr[[]byte](fmt.Errorf("cannot write tar header for %q: %w", path, err))
				return
			}
			log.Printf("wrote tar header for file %q\n", path)
			file, err := os.Open(cleanedPath)
			if err != nil {
				rtn <- wshutil.RespErr[[]byte](fmt.Errorf("cannot open file %q: %w", path, err))
				return
			}
			log.Printf("opened file %q\n", path)
			n, err := file.WriteTo(tarWriter)
			if err != nil {
				rtn <- wshutil.RespErr[[]byte](fmt.Errorf("cannot write file %q to tar stream: %w", path, err))
				return
			}
			log.Printf("wrote %d bytes to tar stream\n", n)
		}
	}()
	log.Printf("returning channel\n")
	return rtn
}

func (impl *ServerImpl) RemoteFileCopyCommand(ctx context.Context, data wshrpc.CommandRemoteFileCopyData) error {
	opts := data.Opts
	destPath := data.DestPath
	srcUri := data.SrcUri
	merge := opts != nil && opts.Merge
	overwrite := opts != nil && opts.Overwrite
	recursive := opts != nil && opts.Recursive
	destPathCleaned := filepath.Clean(wavebase.ExpandHomeDirSafe(destPath))
	destinfo, err := os.Stat(destPathCleaned)
	if err == nil {
		if destinfo.IsDir() {
			if !recursive {
				return fmt.Errorf("destination %q is a directory, use recursive option", destPath)
			}
			if !merge {
				if overwrite {
					err := os.RemoveAll(destPathCleaned)
					if err != nil {
						return fmt.Errorf("cannot remove directory %q: %w", destPath, err)
					}
				} else {
					return fmt.Errorf("destination %q is a directory, use overwrite option", destPath)
				}
			}
		} else {
			if !overwrite {
				return fmt.Errorf("destination %q already exists, use overwrite option", destPath)
			} else {
				err := os.Remove(destPathCleaned)
				if err != nil {
					return fmt.Errorf("cannot remove file %q: %w", destPath, err)
				}
			}
		}
	}
	ioch := wshclient.FileStreamTarCommand(wshclient.GetBareRpcClient(), wshrpc.CommandRemoteStreamTarData{Path: srcUri, Opts: opts}, &wshrpc.RpcOpts{})
	pipeReader, pipeWriter := io.Pipe()
	tarReader := tar.NewReader(pipeReader)
	ctx, cancel := context.WithCancel(ctx)
	iochan.WriterChan(ctx, pipeWriter, ioch)
	defer pipeWriter.Close()
	defer pipeReader.Close()
	defer cancel()
	for next, err := tarReader.Next(); err == nil; {
		finfo := next.FileInfo()
		nextPath := filepath.Clean(filepath.Join(destPathCleaned, next.Name))
		destinfo, err = os.Stat(nextPath)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cannot stat file %q: %w", nextPath, err)
		}

		if destinfo != nil {
			if destinfo.IsDir() {
				if !finfo.IsDir() {
					if !overwrite {
						return fmt.Errorf("cannot create directory %q, file exists at path, overwrite not specified", nextPath)
					} else {
						err := os.Remove(nextPath)
						if err != nil {
							return fmt.Errorf("cannot remove file %q: %w", nextPath, err)
						}
					}
				} else if !merge && !overwrite {
					return fmt.Errorf("cannot create directory %q, directory exists at path, neither overwrite nor merge specified", nextPath)
				} else if overwrite {
					err := os.RemoveAll(nextPath)
					if err != nil {
						return fmt.Errorf("cannot remove directory %q: %w", nextPath, err)
					}
				}
			} else {
				if finfo.IsDir() {
					if !overwrite {
						return fmt.Errorf("cannot create file %q, directory exists at path, overwrite not specified", nextPath)
					} else {
						err := os.RemoveAll(nextPath)
						if err != nil {
							return fmt.Errorf("cannot remove directory %q: %w", nextPath, err)
						}
					}
				} else if !overwrite {
					return fmt.Errorf("cannot create file %q, file exists at path, overwrite not specified", nextPath)
				} else {
					err := os.Remove(nextPath)
					if err != nil {
						return fmt.Errorf("cannot remove file %q: %w", nextPath, err)
					}
				}
			}
		} else {
			if finfo.IsDir() {
				err := os.MkdirAll(nextPath, finfo.Mode())
				if err != nil {
					return fmt.Errorf("cannot create directory %q: %w", nextPath, err)
				}
			} else {
				file, err := os.Create(nextPath)
				if err != nil {
					return fmt.Errorf("cannot create new file %q: %w", nextPath, err)
				}
				_, err = io.Copy(file, tarReader)
				if err != nil {
					return fmt.Errorf("cannot write file %q: %w", nextPath, err)
				}
				file.Chmod(finfo.Mode())
				file.Close()
			}
		}
	}
	if err != nil && err != io.EOF {
		return fmt.Errorf("cannot read tar stream: %w", err)
	}
	return nil
}

func (impl *ServerImpl) RemoteListEntriesCommand(ctx context.Context, data wshrpc.CommandRemoteListEntriesData) chan wshrpc.RespOrErrorUnion[wshrpc.CommandRemoteListEntriesRtnData] {
	ch := make(chan wshrpc.RespOrErrorUnion[wshrpc.CommandRemoteListEntriesRtnData], 16)
	go func() {
		defer close(ch)
		path, err := wavebase.ExpandHomeDir(data.Path)
		if err != nil {
			ch <- wshutil.RespErr[wshrpc.CommandRemoteListEntriesRtnData](err)
			return
		}
		innerFilesEntries := []os.DirEntry{}
		seen := 0
		if data.Opts.Limit == 0 {
			data.Opts.Limit = wshrpc.MaxDirSize
		}
		if data.Opts.All {
			fs.WalkDir(os.DirFS(path), ".", func(path string, d fs.DirEntry, err error) error {
				defer func() {
					seen++
				}()
				if seen < data.Opts.Offset {
					return nil
				}
				if seen >= data.Opts.Offset+data.Opts.Limit {
					return io.EOF
				}
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				innerFilesEntries = append(innerFilesEntries, d)
				return nil
			})
		} else {
			innerFilesEntries, err = os.ReadDir(path)
			if err != nil {
				ch <- wshutil.RespErr[wshrpc.CommandRemoteListEntriesRtnData](fmt.Errorf("cannot open dir %q: %w", path, err))
				return
			}
		}
		var fileInfoArr []*wshrpc.FileInfo
		for _, innerFileEntry := range innerFilesEntries {
			if ctx.Err() != nil {
				ch <- wshutil.RespErr[wshrpc.CommandRemoteListEntriesRtnData](ctx.Err())
				return
			}
			innerFileInfoInt, err := innerFileEntry.Info()
			if err != nil {
				log.Printf("cannot stat file %q: %v\n", innerFileEntry.Name(), err)
				continue
			}
			innerFileInfo := statToFileInfo(filepath.Join(path, innerFileInfoInt.Name()), innerFileInfoInt, false)
			fileInfoArr = append(fileInfoArr, innerFileInfo)
			if len(fileInfoArr) >= wshrpc.DirChunkSize {
				resp := wshrpc.CommandRemoteListEntriesRtnData{FileInfo: fileInfoArr}
				ch <- wshrpc.RespOrErrorUnion[wshrpc.CommandRemoteListEntriesRtnData]{Response: resp}
				fileInfoArr = nil
			}
		}
		if len(fileInfoArr) > 0 {
			resp := wshrpc.CommandRemoteListEntriesRtnData{FileInfo: fileInfoArr}
			ch <- wshrpc.RespOrErrorUnion[wshrpc.CommandRemoteListEntriesRtnData]{Response: resp}
		}
	}()
	return ch
}

func statToFileInfo(fullPath string, finfo fs.FileInfo, extended bool) *wshrpc.FileInfo {
	mimeType := fileutil.DetectMimeType(fullPath, finfo, extended)
	rtn := &wshrpc.FileInfo{
		Path:          wavebase.ReplaceHomeDir(fullPath),
		Dir:           computeDirPart(fullPath, finfo.IsDir()),
		Name:          finfo.Name(),
		Size:          finfo.Size(),
		Mode:          finfo.Mode(),
		ModeStr:       finfo.Mode().String(),
		ModTime:       finfo.ModTime().UnixMilli(),
		IsDir:         finfo.IsDir(),
		MimeType:      mimeType,
		SupportsMkdir: true,
	}
	if finfo.IsDir() {
		rtn.Size = -1
	}
	return rtn
}

// fileInfo might be null
func checkIsReadOnly(path string, fileInfo fs.FileInfo, exists bool) bool {
	if !exists || fileInfo.Mode().IsDir() {
		dirName := filepath.Dir(path)
		randHexStr, err := utilfn.RandomHexString(12)
		if err != nil {
			// we're not sure, just return false
			return false
		}
		tmpFileName := filepath.Join(dirName, "wsh-tmp-"+randHexStr)
		fd, err := os.Create(tmpFileName)
		if err != nil {
			return true
		}
		fd.Close()
		os.Remove(tmpFileName)
		return false
	}
	// try to open for writing, if this fails then it is read-only
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return true
	}
	file.Close()
	return false
}

func computeDirPart(path string, isDir bool) string {
	path = filepath.Clean(wavebase.ExpandHomeDirSafe(path))
	path = filepath.ToSlash(path)
	if path == "/" {
		return "/"
	}
	path = strings.TrimSuffix(path, "/")
	if isDir {
		return path
	}
	return filepath.Dir(path)
}

func (*ServerImpl) fileInfoInternal(path string, extended bool) (*wshrpc.FileInfo, error) {
	cleanedPath := filepath.Clean(wavebase.ExpandHomeDirSafe(path))
	finfo, err := os.Stat(cleanedPath)
	if os.IsNotExist(err) {
		return &wshrpc.FileInfo{
			Path:          wavebase.ReplaceHomeDir(path),
			Dir:           computeDirPart(path, false),
			NotFound:      true,
			ReadOnly:      checkIsReadOnly(cleanedPath, finfo, false),
			SupportsMkdir: true,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot stat file %q: %w", path, err)
	}
	rtn := statToFileInfo(cleanedPath, finfo, extended)
	if extended {
		rtn.ReadOnly = checkIsReadOnly(cleanedPath, finfo, true)
	}
	return rtn, nil
}

func resolvePaths(paths []string) string {
	if len(paths) == 0 {
		return wavebase.ExpandHomeDirSafe("~")
	}
	rtnPath := wavebase.ExpandHomeDirSafe(paths[0])
	for _, path := range paths[1:] {
		path = wavebase.ExpandHomeDirSafe(path)
		if filepath.IsAbs(path) {
			rtnPath = path
			continue
		}
		rtnPath = filepath.Join(rtnPath, path)
	}
	return rtnPath
}

func (impl *ServerImpl) RemoteFileJoinCommand(ctx context.Context, paths []string) (*wshrpc.FileInfo, error) {
	rtnPath := resolvePaths(paths)
	return impl.fileInfoInternal(rtnPath, true)
}

func (impl *ServerImpl) RemoteFileInfoCommand(ctx context.Context, path string) (*wshrpc.FileInfo, error) {
	return impl.fileInfoInternal(path, true)
}

func (impl *ServerImpl) RemoteFileTouchCommand(ctx context.Context, path string) error {
	cleanedPath := filepath.Clean(wavebase.ExpandHomeDirSafe(path))
	if _, err := os.Stat(cleanedPath); err == nil {
		return fmt.Errorf("file %q already exists", path)
	}
	if err := os.MkdirAll(filepath.Dir(cleanedPath), 0755); err != nil {
		return fmt.Errorf("cannot create directory %q: %w", filepath.Dir(cleanedPath), err)
	}
	if err := os.WriteFile(cleanedPath, []byte{}, 0644); err != nil {
		return fmt.Errorf("cannot create file %q: %w", cleanedPath, err)
	}
	return nil
}

func (impl *ServerImpl) RemoteFileRenameCommand(ctx context.Context, pathTuple [2]string) error {
	path := pathTuple[0]
	newPath := pathTuple[1]
	cleanedPath := filepath.Clean(wavebase.ExpandHomeDirSafe(path))
	cleanedNewPath := filepath.Clean(wavebase.ExpandHomeDirSafe(newPath))
	if _, err := os.Stat(cleanedNewPath); err == nil {
		return fmt.Errorf("destination file path %q already exists", path)
	}
	if err := os.Rename(cleanedPath, cleanedNewPath); err != nil {
		return fmt.Errorf("cannot rename file %q to %q: %w", cleanedPath, cleanedNewPath, err)
	}
	return nil
}

func (impl *ServerImpl) RemoteMkdirCommand(ctx context.Context, path string) error {
	cleanedPath := filepath.Clean(wavebase.ExpandHomeDirSafe(path))
	if stat, err := os.Stat(cleanedPath); err == nil {
		if stat.IsDir() {
			return fmt.Errorf("directory %q already exists", path)
		} else {
			return fmt.Errorf("cannot create directory %q, file exists at path", path)
		}
	}
	if err := os.MkdirAll(cleanedPath, 0755); err != nil {
		return fmt.Errorf("cannot create directory %q: %w", cleanedPath, err)
	}
	return nil
}

func (*ServerImpl) RemoteWriteFileCommand(ctx context.Context, data wshrpc.CommandRemoteWriteFileData) error {
	path, err := wavebase.ExpandHomeDir(data.Path)
	if err != nil {
		return err
	}
	createMode := data.CreateMode
	if createMode == 0 {
		createMode = 0644
	}
	dataSize := base64.StdEncoding.DecodedLen(len(data.Data64))
	dataBytes := make([]byte, dataSize)
	n, err := base64.StdEncoding.Decode(dataBytes, []byte(data.Data64))
	if err != nil {
		return fmt.Errorf("cannot decode base64 data: %w", err)
	}
	err = os.WriteFile(path, dataBytes[:n], createMode)
	if err != nil {
		return fmt.Errorf("cannot write file %q: %w", path, err)
	}
	return nil
}

func (*ServerImpl) RemoteFileDeleteCommand(ctx context.Context, path string) error {
	expandedPath, err := wavebase.ExpandHomeDir(path)
	if err != nil {
		return fmt.Errorf("cannot delete file %q: %w", path, err)
	}
	cleanedPath := filepath.Clean(expandedPath)
	err = os.Remove(cleanedPath)
	if err != nil {
		return fmt.Errorf("cannot delete file %q: %w", path, err)
	}
	return nil
}

func (*ServerImpl) RemoteGetInfoCommand(ctx context.Context) (wshrpc.RemoteInfo, error) {
	return wshutil.GetInfo(), nil
}

func (*ServerImpl) RemoteInstallRcFilesCommand(ctx context.Context) error {
	return wshutil.InstallRcFiles()
}
