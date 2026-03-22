use discord_rich_presence::{activity, DiscordIpc, DiscordIpcClient};
use mdns_sd::{ServiceDaemon, ServiceEvent};
use serde::Serialize;
use std::{
    sync::{mpsc, Mutex},
    time::{Duration, Instant},
};
use tauri::{
    menu::{Menu, MenuItem},
    tray::TrayIconBuilder,
    AppHandle, Emitter, Manager, State,
};

// ─── mDNS Discovery ──────────────────────────────────────────────────────────

#[derive(Debug, Serialize)]
pub struct DiscoveredServer {
    pub name: String,
    pub host: String,
    pub port: u16,
    pub url: String,
    pub version: String,
}

#[tauri::command]
async fn discover_servers() -> Vec<DiscoveredServer> {
    let mdns = match ServiceDaemon::new() {
        Ok(d) => d,
        Err(_) => return vec![],
    };

    let receiver = match mdns.browse("_orb._tcp.local.") {
        Ok(r) => r,
        Err(_) => return vec![],
    };

    let mut servers = Vec::new();
    let deadline = Instant::now() + Duration::from_secs(3);

    while Instant::now() < deadline {
        let remaining = deadline.saturating_duration_since(Instant::now());
        match receiver.recv_timeout(remaining.min(Duration::from_millis(100))) {
            Ok(ServiceEvent::ServiceResolved(info)) => {
                let host = info
                    .get_addresses()
                    .iter()
                    .next()
                    .map(|a| a.to_string())
                    .unwrap_or_else(|| info.get_hostname().trim_end_matches('.').to_string());
                let port = info.get_port();
                let url = format!("http://{}:{}/api", host, port);
                servers.push(DiscoveredServer {
                    name: info.get_fullname().to_string(),
                    host,
                    port,
                    url,
                    version: String::new(),
                });
            }
            Ok(_) => {}
            Err(_) => break,
        }
    }

    mdns.shutdown().ok();
    servers
}

// ─── Discord Rich Presence ────────────────────────────────────────────────────

struct DiscordState(Mutex<Option<DiscordIpcClient>>);

#[tauri::command]
fn discord_connect(state: State<'_, DiscordState>) -> bool {
    let mut guard = state.0.lock().unwrap();
    let app_id = option_env!("DISCORD_APP_ID").unwrap_or("1485330037191213260");
    let mut client = DiscordIpcClient::new(app_id);
    if client.connect().is_err() {
        return false;
    }
    *guard = Some(client);
    true
}

#[tauri::command]
fn discord_update(title: String, artist: String, _album: String, state: State<'_, DiscordState>) {
    let mut guard = state.0.lock().unwrap();
    if let Some(client) = guard.as_mut() {
        let details = title.clone();
        let state_text = artist.clone();
        let payload = activity::Activity::new()
            .details(&details)
            .state(&state_text);
        client.set_activity(payload).ok();
    }
}

#[tauri::command]
fn discord_clear(state: State<'_, DiscordState>) {
    let mut guard = state.0.lock().unwrap();
    if let Some(client) = guard.as_mut() {
        client.clear_activity().ok();
    }
}

#[tauri::command]
fn discord_disconnect(state: State<'_, DiscordState>) {
    let mut guard = state.0.lock().unwrap();
    if let Some(mut client) = guard.take() {
        client.close().ok();
    }
}

// ─── Native Media Controls (SMTC / MPRIS / MediaRemote via souvlaki) ─────────

enum MediaCmd {
    Update {
        title: String,
        artist: String,
        album: String,
        cover_url: Option<String>,
        duration_ms: u64,
    },
    SetPlayback {
        playing: bool,
        position_ms: u64,
    },
    Clear,
}

/// Holds the sender end of the channel to the souvlaki media-controls thread.
/// `None` when souvlaki is not available for the current platform.
struct NativeMediaSender(Mutex<Option<mpsc::Sender<MediaCmd>>>);

#[cfg(any(target_os = "windows", target_os = "macos", target_os = "linux"))]
fn start_media_controls_thread(app: AppHandle, hwnd_raw: Option<usize>) -> mpsc::Sender<MediaCmd> {
    use souvlaki::{
        MediaControlEvent, MediaControls, MediaMetadata, MediaPlayback, MediaPosition,
        PlatformConfig,
    };

    let (tx, rx) = mpsc::channel::<MediaCmd>();

    std::thread::spawn(move || {
        let config = PlatformConfig {
            dbus_name: "org.orb.music",
            display_name: "Orb",
            #[cfg(target_os = "windows")]
            hwnd: hwnd_raw.map(|h| h as *mut std::ffi::c_void),
        };

        let mut controls = match MediaControls::new(config) {
            Ok(c) => c,
            Err(_) => return,
        };

        let app_clone = app.clone();
        let _ = controls.attach(move |event: MediaControlEvent| match event {
            MediaControlEvent::Play => {
                app_clone.emit("smtc-play", ()).ok();
            }
            MediaControlEvent::Pause => {
                app_clone.emit("smtc-pause", ()).ok();
            }
            MediaControlEvent::Toggle => {
                app_clone.emit("smtc-toggle", ()).ok();
            }
            MediaControlEvent::Next => {
                app_clone.emit("smtc-next", ()).ok();
            }
            MediaControlEvent::Previous => {
                app_clone.emit("smtc-previous", ()).ok();
            }
            _ => {}
        });

        while let Ok(cmd) = rx.recv() {
            match cmd {
                MediaCmd::Update {
                    title,
                    artist,
                    album,
                    cover_url,
                    duration_ms,
                } => {
                    let duration = (duration_ms > 0)
                        .then(|| std::time::Duration::from_millis(duration_ms));
                    let _ = controls.set_metadata(MediaMetadata {
                        title: Some(&title),
                        artist: Some(&artist),
                        album: if album.is_empty() { None } else { Some(&album) },
                        cover_url: cover_url.as_deref(),
                        duration,
                    });
                }
                MediaCmd::SetPlayback {
                    playing,
                    position_ms,
                } => {
                    let progress =
                        Some(MediaPosition(std::time::Duration::from_millis(position_ms)));
                    let playback = if playing {
                        MediaPlayback::Playing { progress }
                    } else {
                        MediaPlayback::Paused { progress }
                    };
                    let _ = controls.set_playback(playback);
                }
                MediaCmd::Clear => {
                    let _ = controls.set_playback(MediaPlayback::Stopped);
                }
            }
        }
    });

    tx
}

#[tauri::command]
fn native_media_update(
    title: String,
    artist: String,
    album: String,
    cover_url: Option<String>,
    duration_ms: u64,
    state: State<'_, NativeMediaSender>,
) {
    if let Some(tx) = state.0.lock().unwrap().as_ref() {
        tx.send(MediaCmd::Update {
            title,
            artist,
            album,
            cover_url,
            duration_ms,
        })
        .ok();
    }
}

#[tauri::command]
fn native_media_playback(
    playing: bool,
    position_ms: u64,
    state: State<'_, NativeMediaSender>,
) {
    if let Some(tx) = state.0.lock().unwrap().as_ref() {
        tx.send(MediaCmd::SetPlayback { playing, position_ms }).ok();
    }
}

#[tauri::command]
fn native_media_clear(state: State<'_, NativeMediaSender>) {
    if let Some(tx) = state.0.lock().unwrap().as_ref() {
        tx.send(MediaCmd::Clear).ok();
    }
}

// ─── System Tray ─────────────────────────────────────────────────────────────

struct PlayPauseItem(Mutex<MenuItem<tauri::Wry>>);

#[tauri::command]
fn set_tray_playback_state(playing: bool, state: State<'_, PlayPauseItem>) {
    let item = state.0.lock().unwrap();
    item.set_text(if playing { "Pause" } else { "Play" }).ok();
}

fn setup_tray(app: &AppHandle) -> tauri::Result<()> {
    let previous = MenuItem::with_id(app, "previous", "Previous", true, None::<&str>)?;
    let play_pause = MenuItem::with_id(app, "play_pause", "Play", true, None::<&str>)?;
    let next = MenuItem::with_id(app, "next", "Next", true, None::<&str>)?;
    let separator = tauri::menu::PredefinedMenuItem::separator(app)?;
    let quit = MenuItem::with_id(app, "quit", "Quit Orb", true, None::<&str>)?;

    let menu = Menu::with_items(app, &[&previous, &play_pause, &next, &separator, &quit])?;

    // Store a reference to the play/pause item so we can update its label.
    app.manage(PlayPauseItem(Mutex::new(play_pause)));

    TrayIconBuilder::new()
        .icon(app.default_window_icon().cloned().unwrap())
        .menu(&menu)
        .tooltip("Orb")
        .on_menu_event(|app, event| match event.id.as_ref() {
            "previous" => {
                app.emit("tray-previous", ()).ok();
            }
            "play_pause" => {
                app.emit("tray-play-pause", ()).ok();
            }
            "next" => {
                app.emit("tray-next", ()).ok();
            }
            "quit" => {
                app.exit(0);
            }
            _ => {}
        })
        .build(app)?;

    Ok(())
}

// ─── Desktop Setup ───────────────────────────────────────────────────────────

pub fn setup(builder: tauri::Builder<tauri::Wry>) -> tauri::Builder<tauri::Wry> {
    builder
        .manage(DiscordState(Mutex::new(None)))
        .manage(NativeMediaSender(Mutex::new(None)))
        .setup(|app| {
            setup_tray(&app.handle())?;

            // Start the native media controls thread (souvlaki) on supported platforms.
            #[cfg(any(target_os = "windows", target_os = "macos", target_os = "linux"))]
            {
                #[cfg(target_os = "windows")]
                let hwnd_raw: Option<usize> = app
                    .get_webview_window("main")
                    .and_then(|w| w.hwnd().ok())
                    .map(|h| h.0 as usize);

                #[cfg(not(target_os = "windows"))]
                let hwnd_raw: Option<usize> = None;

                let tx = start_media_controls_thread(app.handle().clone(), hwnd_raw);
                *app.state::<NativeMediaSender>().0.lock().unwrap() = Some(tx);
            }

            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            discover_servers,
            discord_connect,
            discord_update,
            discord_clear,
            discord_disconnect,
            set_tray_playback_state,
            native_media_update,
            native_media_playback,
            native_media_clear,
        ])
}
