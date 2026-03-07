use discord_rich_presence::{activity, DiscordIpc, DiscordIpcClient};
use mdns_sd::{ServiceDaemon, ServiceEvent};
use serde::Serialize;
use std::{
    sync::Mutex,
    time::{Duration, Instant},
};
use tauri::{
    menu::{Menu, MenuItem},
    tray::TrayIconBuilder,
    AppHandle, Manager, State, Emitter,
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
    // Client ID — replace with your application's ID if you have one.
    let mut client = DiscordIpcClient::new("1234567890");
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

// ─── Entry Point ─────────────────────────────────────────────────────────────

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .manage(DiscordState(Mutex::new(None)))
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
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
