import * as React from "react";
import * as mobxReact from "mobx-react";
import * as mobx from "mobx";
import {sprintf} from "sprintf-js";
import {boundMethod} from "autobind-decorator";
import {If, For, When, Otherwise, Choose} from "tsx-control-statements/components";
import cn from "classnames";
import {GlobalModel, GlobalCommandRunner, TabColors} from "./model";
import {Toggle} from "./elements";
import {LineType, RendererPluginType, ClientDataType} from "./types";

type OV<V> = mobx.IObservableValue<V>;
type OArr<V> = mobx.IObservableArray<V>;
type OMap<K,V> = mobx.ObservableMap<K,V>;
type CV<V> = mobx.IComputedValue<V>;

// @ts-ignore
const VERSION = __PROMPT_VERSION__;
// @ts-ignore
const BUILD = __PROMPT_BUILD__;

@mobxReact.observer
class ScreenSettingsModal extends React.Component<{sessionId : string, screenId : string}, {}> {
    tempName : OV<string>;
    tempTabColor : OV<string>;
    tempArchived : OV<boolean>;

    constructor(props : any) {
        super(props);
        let {sessionId, screenId} = props;
        let screen = GlobalModel.getScreenById(sessionId, screenId);
        if (screen == null) {
            return;
        }
        this.tempName = mobx.observable.box(screen.name.get(), {name: "screenSettings-tempName"});
        this.tempTabColor = mobx.observable.box(screen.getTabColor(), {name: "screenSettings-tempTabColor"});
        this.tempArchived = mobx.observable.box(screen.archived.get(), {name: "screenSettings-tempArchived"});
    }
    
    @boundMethod
    closeModal() : void {
        mobx.action(() => {
            GlobalModel.screenSettingsModal.set(null);
        })();
    }

    @boundMethod
    handleOK() : void {
        mobx.action(() => {
            GlobalModel.screenSettingsModal.set(null);
        })();
        let screen = GlobalModel.getScreenById(this.props.sessionId, this.props.screenId);
        if (screen == null) {
            return;
        }
        let settings : {tabcolor? : string, name? : string} = {};
        if (this.tempTabColor.get() != screen.getTabColor()) {
            settings.tabcolor = this.tempTabColor.get();
        }
        if (this.tempName.get() != screen.name.get()) {
            settings.name = this.tempName.get();
        }
        if (Object.keys(settings).length > 0) {
            GlobalCommandRunner.screenSetSettings(this.props.screenId, settings);
        }
        if (this.tempArchived.get() != screen.archived.get()) {
            GlobalCommandRunner.screenArchive(screen.screenId, this.tempArchived.get());
        }
    }

    @boundMethod
    handleChangeName(e : any) : void {
        mobx.action(() => {
            this.tempName.set(e.target.value);
        })();
    }

    @boundMethod
    selectTabColor(color : string) : void {
        mobx.action(() => {
            this.tempTabColor.set(color);
        })();
    }

    @boundMethod
    handleChangeArchived(val : boolean) : void {
        mobx.action(() => {
            this.tempArchived.set(val);
        })();
    }

    render() {
        let {sessionId, screenId} = this.props;
        let screen = GlobalModel.getScreenById(sessionId, screenId);
        if (screen == null) {
            return null;
        }
        let color : string = null;
        return (
            <div className={cn("modal screen-settings-modal settings-modal prompt-modal is-active")}>
                <div className="modal-background"/>
                <div className="modal-content">
                    <header>
                        <div className="modal-title">screen settings ({screen.name.get()})</div>
                        <div className="close-icon">
                            <i onClick={this.closeModal} className="fa-sharp fa-solid fa-times"/>
                        </div>
                    </header>
                    <div className="inner-content">
                        <div className="settings-field">
                            <div className="settings-label">
                                Name
                            </div>
                            <div className="settings-input">
                                <input type="text" placeholder="Tab Name" onChange={this.handleChangeName} value={this.tempName.get()} maxLength={50}/>
                            </div>
                        </div>
                        <div className="settings-field">
                            <div className="settings-label">
                                Tab Color
                            </div>
                            <div className="settings-input">
                                <div className="tab-colors">
                                    <div className="tab-color-cur">
                                        <span className={cn("icon tab-color-icon", "color-" + this.tempTabColor.get())}>
                                            <i className="fa-sharp fa-solid fa-square"/>
                                        </span>
                                        <span>{this.tempTabColor.get()}</span>
                                    </div>
                                    <div className="tab-color-sep">|</div>
                                    <For each="color" of={TabColors}>
                                        <div key={color} className="tab-color-select" onClick={() => this.selectTabColor(color)}>
                                            <span className={cn("tab-color-icon", "color-" + color)}>
                                                <i className="fa-sharp fa-solid fa-square"/>
                                            </span>
                                        </div>
                                    </For>
                                </div>
                            </div>
                        </div>
                        <div className="settings-field">
                            <div className="settings-label">
                                Archived
                            </div>
                            <div className="settings-input">
                                <Toggle checked={this.tempArchived.get()} onChange={this.handleChangeArchived}/>
                                <div className="action-text">
                                    <If condition={this.tempArchived.get() && this.tempArchived.get() != screen.archived.get()}>will be archived</If>
                                    <If condition={!this.tempArchived.get() && this.tempArchived.get() != screen.archived.get()}>will be un-archived</If>
                                </div>
                            </div>
                        </div>
                    </div>
                    <footer>
                        <div onClick={this.closeModal} className="button is-prompt-cancel is-outlined is-small">Cancel</div>
                        <div onClick={this.handleOK} className="button is-prompt-green is-outlined is-small">OK</div>
                    </footer>
                </div>
            </div>
        );
    }
}

@mobxReact.observer
class SessionSettingsModal extends React.Component<{sessionId : string}, {}> {
    tempName : OV<string>;

    constructor(props : any) {
        super(props);
        let {sessionId} = props;
        let session = GlobalModel.getSessionById(sessionId);
        if (session == null) {
            return;
        }
        this.tempName = mobx.observable.box(session.name.get(), {name: "sessionSettings-tempName"});
    }
    
    @boundMethod
    closeModal() : void {
        mobx.action(() => {
            GlobalModel.sessionSettingsModal.set(null);
        })();
    }

    @boundMethod
    handleOK() : void {
        mobx.action(() => {
            GlobalModel.sessionSettingsModal.set(null);
            GlobalCommandRunner.sessionSetSettings(this.props.sessionId, {
                "name": this.tempName.get(),
            });
        })();
    }

    @boundMethod
    handleChangeName(e : any) : void {
        mobx.action(() => {
            this.tempName.set(e.target.value);
        })();
    }

    render() {
        let {sessionId} = this.props;
        let session = GlobalModel.getSessionById(sessionId);
        if (session == null) {
            return null;
        }
        return (
            <div className={cn("modal session-settings-modal settings-modal prompt-modal is-active")}>
                <div className="modal-background"/>
                <div className="modal-content">
                    <header>
                        <div className="modal-title">session settings ({session.name.get()})</div>
                        <div className="close-icon">
                            <i onClick={this.closeModal} className="fa-sharp fa-solid fa-times"/>
                        </div>
                    </header>
                    <div className="inner-content">
                        <div className="settings-field">
                            <div className="settings-label">
                                Name
                            </div>
                            <div className="settings-input">
                                <input type="text" placeholder="Tab Name" onChange={this.handleChangeName} value={this.tempName.get()} maxLength={50}/>
                            </div>
                        </div>
                    </div>
                    <footer>
                        <div onClick={this.closeModal} className="button is-prompt-cancel is-outlined is-small">Cancel</div>
                        <div onClick={this.handleOK} className="button is-prompt-green is-outlined is-small">OK</div>
                    </footer>
                </div>
            </div>
        );
    }
}

@mobxReact.observer
class LineSettingsModal extends React.Component<{line : LineType}, {}> {
    tempArchived : OV<boolean>;
    tempRenderer : OV<string>;
    rendererDropdownActive : OV<boolean> = mobx.observable.box(false, {name: "lineSettings-rendererDropdownActive"});
    
    constructor(props : any) {
        super(props);
        let {line} = props;
        if (line == null) {
            return;
        }
        this.tempArchived = mobx.observable.box(!!line.archived, {name: "lineSettings-tempArchived"});
        this.tempRenderer = mobx.observable.box(line.renderer, {name: "lineSettings-renderer"});
    }
    
    @boundMethod
    closeModal() : void {
        mobx.action(() => {
            GlobalModel.lineSettingsModal.set(null);
        })();
    }

    @boundMethod
    handleOK() : void {
        let {line} = this.props;
        mobx.action(() => {
            GlobalModel.lineSettingsModal.set(null);
        })();
        if (this.tempRenderer.get() != line.renderer) {
            GlobalCommandRunner.lineSet(line.lineid, {
                "renderer": this.tempRenderer.get(),
            });
        }
        if (this.tempArchived.get() != !!line.archived) {
            GlobalCommandRunner.lineArchive(line.lineid, this.tempArchived.get());
        }
    }

    @boundMethod
    handleChangeArchived(val : boolean) : void {
        mobx.action(() => {
            this.tempArchived.set(val);
        })();
    }

    @boundMethod
    toggleRendererDropdown() : void {
        mobx.action(() => {
            this.rendererDropdownActive.set(!this.rendererDropdownActive.get());
        })();
    }

    @boundMethod
    clickSetRenderer(renderer : string) : void {
        mobx.action(() => {
            this.tempRenderer.set(renderer);
            this.rendererDropdownActive.set(false);
        })();
    }

    renderRendererDropdown() : any {
        let {line} = this.props;
        let renderer = this.tempRenderer.get() ?? "terminal";
        let plugins = GlobalModel.rendererPlugins;
        let plugin : RendererPluginType = null;
        return (
            <div className={cn("dropdown", "renderer-dropdown", {"is-active": this.rendererDropdownActive.get()})}>
                <div className="dropdown-trigger">
                    <button onClick={this.toggleRendererDropdown} className="button is-small is-dark">
                        <span><i className="fa-sharp fa-solid fa-fill"/> {renderer}</span>
                        <span className="icon is-small">
                            <i className="fa-sharp fa-regular fa-angle-down" aria-hidden="true"></i>
                        </span>
                    </button>
                </div>
                <div className="dropdown-menu" role="menu">
                    <div className="dropdown-content has-background-black">
                        <div onClick={() => this.clickSetRenderer("none") } key="none" className="dropdown-item">none</div>
                        <div onClick={() => this.clickSetRenderer(null) } key="terminal" className="dropdown-item">terminal</div>
                        <For each="plugin" of={plugins}>
                            <div onClick={() => this.clickSetRenderer(plugin.name) } key={plugin.name} className="dropdown-item">{plugin.name}</div>
                        </For>
                    </div>
                </div>
            </div>
        );
    }

    render() {
        let {line} = this.props;
        if (line == null) {
            return null;
        }
        return (
            <div className={cn("modal line-settings-modal settings-modal prompt-modal is-active")}>
                <div className="modal-background"/>
                <div className="modal-content">
                    <header>
                        <div className="modal-title">line settings ({line.linenum})</div>
                        <div className="close-icon">
                            <i onClick={this.closeModal} className="fa-sharp fa-solid fa-times"/>
                        </div>
                    </header>
                    <div className="inner-content">
                        <div className="settings-field">
                            <div className="settings-label">
                                Renderer
                            </div>
                            <div className="settings-input">
                                {this.renderRendererDropdown()}
                            </div>
                        </div>
                        <div className="settings-field">
                            <div className="settings-label">
                                Archived
                            </div>
                            <div className="settings-input">
                                <Toggle checked={this.tempArchived.get()} onChange={this.handleChangeArchived}/>
                                <div className="action-text">
                                    <If condition={this.tempArchived.get() && this.tempArchived.get() != !!line.archived}>will be archived</If>
                                    <If condition={!this.tempArchived.get() && this.tempArchived.get() != !!line.archived}>will be un-archived</If>
                                </div>
                            </div>
                        </div>
                        <div style={{height: 50}}/>
                    </div>
                    <footer>
                        <div onClick={this.closeModal} className="button is-prompt-cancel is-outlined is-small">Cancel</div>
                        <div onClick={this.handleOK} className="button is-prompt-green is-outlined is-small">OK</div>
                    </footer>
                </div>
            </div>
        );
    }
}

@mobxReact.observer
class ClientSettingsModal extends React.Component<{}, {}> {
    tempFontSize : OV<number>;
    tempTelemetry : OV<boolean>;
    fontSizeDropdownActive : OV<boolean> = mobx.observable.box(false, {name: "clientSettings-fontSizeDropdownActive"});

    constructor(props : any) {
        super(props);
        let cdata = GlobalModel.clientData.get();
        this.tempFontSize = mobx.observable.box(GlobalModel.termFontSize.get(), {name: "clientSettings-tempFontSize"});
        this.tempTelemetry = mobx.observable.box(!cdata.clientopts.notelemetry, {name: "clientSettings-telemetry"});
    }
    
    @boundMethod
    closeModal() : void {
        mobx.action(() => {
            GlobalModel.clientSettingsModal.set(false);
        })();
    }

    @boundMethod
    handleOK() : void {
        mobx.action(() => {
            GlobalModel.clientSettingsModal.set(false);
        })();
        let cdata = GlobalModel.clientData.get();
        let curTel = !cdata.clientopts.notelemetry;
        if (this.tempTelemetry.get() != curTel) {
            if (this.tempTelemetry.get()) {
                GlobalCommandRunner.telemetryOn();
            }
            else {
                GlobalCommandRunner.telemetryOff();
            }
        }
        if (GlobalModel.termFontSize.get() != this.tempFontSize.get()) {
            GlobalCommandRunner.setTermFontSize(this.tempFontSize.get());
        }
    }

    @boundMethod
    handleChangeFontSize(newFontSize : number) : void {
        mobx.action(() => {
            this.fontSizeDropdownActive.set(false);
            this.tempFontSize.set(newFontSize);
        })();
    }

    @boundMethod
    togglefontSizeDropdown() : void {
        mobx.action(() => {
            this.fontSizeDropdownActive.set(!this.fontSizeDropdownActive.get());
        })();
    }

    @boundMethod
    handleChangeTelemetry(val : boolean) : void {
        mobx.action(() => {
            this.tempTelemetry.set(val);
        })();
    }

    renderFontSizeDropdown() : any {
        let availableFontSizes = [8, 9, 10, 11, 12, 13, 14, 15];
        let fsize : number = 0;
        return (
            <div className={cn("dropdown", "font-size-dropdown", {"is-active": this.fontSizeDropdownActive.get()})}>
                <div className="dropdown-trigger">
                    <button onClick={this.togglefontSizeDropdown} className="button is-small is-dark">
                        <span>{this.tempFontSize.get()}px</span>
                        <span className="icon is-small">
                            <i className="fa-sharp fa-regular fa-angle-down" aria-hidden="true"></i>
                        </span>
                    </button>
                </div>
                <div className="dropdown-menu" role="menu">
                    <div className="dropdown-content has-background-black">
                        <For each="fsize" of={availableFontSizes}>
                            <div onClick={() => this.handleChangeFontSize(fsize) } key={fsize + "px"} className="dropdown-item">{fsize}px</div>
                        </For>
                    </div>
                </div>
            </div>
        );
    }

    render() {
        let cdata : ClientDataType = GlobalModel.clientData.get();
        return (
            <div className={cn("modal client-settings-modal settings-modal prompt-modal is-active")}>
                <div className="modal-background"/>
                <div className="modal-content">
                    <header>
                        <div className="modal-title">client settings</div>
                        <div className="close-icon">
                            <i onClick={this.closeModal} className="fa-sharp fa-solid fa-times"/>
                        </div>
                    </header>
                    <div className="inner-content">
                        <div className="settings-field">
                            <div className="settings-label">
                                Term Font Size
                            </div>
                            <div className="settings-input">
                                {this.renderFontSizeDropdown()}
                            </div>
                        </div>
                        <div className="settings-field">
                            <div className="settings-label">
                                Client ID
                            </div>
                            <div className="settings-input">
                                {cdata.clientid}
                            </div>
                        </div>
                        <div className="settings-field">
                            <div className="settings-label">
                                Client Version
                            </div>
                            <div className="settings-input">
                                {VERSION} {BUILD}
                            </div>
                        </div>
                        <div className="settings-field">
                            <div className="settings-label">
                                DB Version
                            </div>
                            <div className="settings-input">
                                {cdata.dbversion}
                            </div>
                        </div>
                        <div className="settings-field">
                            <div className="settings-label">
                                Basic Telemetry
                            </div>
                            <div className="settings-input">
                                <Toggle checked={this.tempTelemetry.get()} onChange={this.handleChangeTelemetry}/>
                            </div>
                        </div>
                    </div>
                    <footer>
                        <div onClick={this.closeModal} className="button is-prompt-cancel is-outlined is-small">Cancel</div>
                        <div onClick={this.handleOK} className="button is-prompt-green is-outlined is-small">OK</div>
                    </footer>
                </div>
            </div>
        );
    }
}

export {ScreenSettingsModal, SessionSettingsModal, LineSettingsModal, ClientSettingsModal};
