use discord_rich_presence::{activity, activity::ActivityType, DiscordIpc, DiscordIpcClient};
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
fn discord_connect(state: State<'_, DiscordState>) -> Result<(), String> {
    let mut guard = state.0.lock().unwrap();
    let app_id = option_env!("DISCORD_APP_ID").unwrap_or("1485330037191213260");
    let mut client = DiscordIpcClient::new(app_id);
    match client.connect() {
        Ok(_) => {
            *guard = Some(client);
            Ok(())
        }
        Err(e) => Err(format!("Discord IPC connect failed: {e:?}")),
    }
}

#[tauri::command]
fn discord_update(
    title: String,
    artist: String,
    album: String,
    playing: bool,
    cover_url: Option<String>,
    state: State<'_, DiscordState>,
) {
    let mut guard = state.0.lock().unwrap();
    // Auto-reconnect if the client was never connected or dropped.
    if guard.is_none() {
        let app_id = option_env!("DISCORD_APP_ID").unwrap_or("1485330037191213260");
        let mut client = DiscordIpcClient::new(app_id);
        if let Ok(_) = client.connect() {
            *guard = Some(client);
        }
    }
    if let Some(client) = guard.as_mut() {
        // When paused, clear presence — Listening type has no pause state.
        if !playing {
            client.clear_activity().ok();
            return;
        }

        // Album art: prefer cover URL (works if server is HTTPS and publicly reachable).
        // Falls back to the "orb" asset key uploaded in the Discord Developer Portal.
        let mut assets = activity::Assets::new();
        assets = match cover_url.as_deref() {
            Some(url) if url.starts_with("https://") => assets.large_url(url).large_image(url),
            _ => assets.large_image("orb"),
        };

        // Note: timestamps are not rendered as a progress bar for Listening activities
        // (that is exclusive to Spotify's native Discord integration).
        let act = activity::Activity::new()
            .details(&title)
            .state(&artist)
            .assets(assets)
            .activity_type(ActivityType::Listening);

        if let Err(e) = client.set_activity(act) {
            *guard = None;
            eprintln!("[Discord] set_activity failed: {e:?}");
        }
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
    let devtools = MenuItem::with_id(app, "devtools", "Open DevTools", true, None::<&str>)?;
    let sep2 = tauri::menu::PredefinedMenuItem::separator(app)?;
    let quit = MenuItem::with_id(app, "quit", "Quit Orb", true, None::<&str>)?;

    let menu = Menu::with_items(app, &[&previous, &play_pause, &next, &separator, &devtools, &sep2, &quit])?;

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
            "devtools" => {
                if let Some(window) = app.get_webview_window("main") {
                    window.open_devtools();
                }
            }
            "quit" => {
                app.exit(0);
            }
            _ => {}
        })
        .build(app)?;

    Ok(())
}

// ─── DevTools ────────────────────────────────────────────────────────────────

#[tauri::command]
fn open_devtools(window: tauri::WebviewWindow) {
    window.open_devtools();
}

// ─── Desktop Setup ───────────────────────────────────────────────────────────

pub fn setup(builder: tauri::Builder<tauri::Wry>) -> tauri::Builder<tauri::Wry> {
    builder
        .manage(DiscordState(Mutex::new(None)))
        .manage(NativeMediaSender(Mutex::new(None)))
        .setup(|app| {
            setup_tray(&app.handle())?;
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
            open_devtools,
        ])
}
