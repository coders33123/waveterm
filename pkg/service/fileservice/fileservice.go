package fileservice

import (
	"context"
	"fmt"
	"time"

	"github.com/wavetermdev/waveterm/pkg/filestore"
	"github.com/wavetermdev/waveterm/pkg/remote/fileshare"
	"github.com/wavetermdev/waveterm/pkg/tsgen/tsgenmeta"
	"github.com/wavetermdev/waveterm/pkg/wconfig"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

const MaxFileSize = 10 * 1024 * 1024 // 10M
const DefaultTimeout = 2 * time.Second

type FileService struct{}

func (fs *FileService) SaveFile_Meta() tsgenmeta.MethodMeta {
	return tsgenmeta.MethodMeta{
		Desc:     "save file",
		ArgNames: []string{"ctx", "connection", "path", "data64"},
	}
}

func (fs *FileService) SaveFile(ctx context.Context, connection string, path string, data64 string) error {
	if connection == "" {
		connection = wshrpc.LocalConnName
	}
	fsclient := fileshare.CreateFileShareClient(ctx, connection)
	return fsclient.PutFile(path, data64)
}

func (fs *FileService) StatFile_Meta() tsgenmeta.MethodMeta {
	return tsgenmeta.MethodMeta{
		Desc:     "get file info",
		ArgNames: []string{"ctx", "connection", "path"},
	}
}

func (fs *FileService) StatFile(ctx context.Context, connection string, path string) (*wshrpc.FileInfo, error) {
	if connection == "" {
		connection = wshrpc.LocalConnName
	}
	fsclient := fileshare.CreateFileShareClient(ctx, connection)
	return fsclient.Stat(path)
}

func (fs *FileService) Mkdir(ctx context.Context, connection string, path string) error {
	if connection == "" {
		connection = wshrpc.LocalConnName
	}
	fsclient := fileshare.CreateFileShareClient(ctx, connection)
	return fsclient.Mkdir(path)
}

func (fs *FileService) TouchFile(ctx context.Context, connection string, path string) error {
	if connection == "" {
		connection = wshrpc.LocalConnName
	}
	fsclient := fileshare.CreateFileShareClient(ctx, connection)
	return fsclient.PutFile(path, "")
}

func (fs *FileService) Rename(ctx context.Context, connection string, path string, newPath string) error {
	if connection == "" {
		connection = wshrpc.LocalConnName
	}
	fsclient := fileshare.CreateFileShareClient(ctx, connection)
	return fsclient.Move(path, newPath, false)
}

func (fs *FileService) ReadFile_Meta() tsgenmeta.MethodMeta {
	return tsgenmeta.MethodMeta{
		Desc:     "read file",
		ArgNames: []string{"ctx", "connection", "path"},
	}
}

func (fs *FileService) ReadFile(ctx context.Context, connection string, path string) (*fileshare.FullFile, error) {
	if connection == "" {
		connection = wshrpc.LocalConnName
	}
	fsclient := fileshare.CreateFileShareClient(ctx, connection)
	return fsclient.Read(path)
}

func (fs *FileService) GetWaveFile(id string, path string) (any, error) {
	ctx, cancelFn := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancelFn()
	file, err := filestore.WFS.Stat(ctx, id, path)
	if err != nil {
		return nil, fmt.Errorf("error getting file: %w", err)
	}
	return file, nil
}

func (fs *FileService) DeleteFile_Meta() tsgenmeta.MethodMeta {
	return tsgenmeta.MethodMeta{
		Desc:     "delete file",
		ArgNames: []string{"ctx", "connection", "path"},
	}
}

func (fs *FileService) DeleteFile(ctx context.Context, connection string, path string) error {
	if connection == "" {
		connection = wshrpc.LocalConnName
	}
	fsclient := fileshare.CreateFileShareClient(ctx, connection)
	return fsclient.Delete(path)
}

func (fs *FileService) GetFullConfig() wconfig.FullConfigType {
	watcher := wconfig.GetWatcher()
	return watcher.GetFullConfig()
}
