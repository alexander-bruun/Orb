use mdns_sd::{ServiceDaemon, ServiceEvent};
use serde::Serialize;
use std::sync::Mutex;
use std::time::Duration;
#[cfg(desktop)]
use tauri::{
    menu::{Menu, MenuItem},
    tray::TrayIconBuilder,
};
use tauri::{Emitter, Manager};

// ---- mDNS discovery ----

#[derive(Debug, Clone, Serialize)]
pub struct DiscoveredServer {
    pub name: String,
    pub host: String,
    pub port: u16,
    pub url: String,
    pub version: String,
}

#[tauri::command]
async fn discover_servers() -> Result<Vec<DiscoveredServer>, String> {
    let mdns = ServiceDaemon::new().map_err(|e| format!("mdns init: {e}"))?;
    let receiver = mdns
        .browse("_orb._tcp.local.")
        .map_err(|e| format!("mdns browse: {e}"))?;

    let mut servers: Vec<DiscoveredServer> = Vec::new();
    let deadline = tokio::time::Instant::now() + Duration::from_secs(3);

    loop {
        let remaining = deadline.saturating_duration_since(tokio::time::Instant::now());
        if remaining.is_zero() {
            break;
        }

        let rx = receiver.clone();
        let wait = remaining.min(Duration::from_millis(500));
        match tokio::task::spawn_blocking(move || rx.recv_timeout(wait)).await {
            Ok(Ok(ServiceEvent::ServiceResolved(info))) => {
                let host = info
                    .get_addresses()
                    .iter()
                    .next()
                    .map(|a| a.to_string())
                    .unwrap_or_else(|| {
                        info.get_hostname().trim_end_matches('.').to_string()
                    });
                let port = info.get_port();

                let props = info.get_properties();
                let path = props
                    .get("path")
                    .map(|v| v.val_str().to_string())
                    .unwrap_or_else(|| "/".to_string());
                let version = props
                    .get("version")
                    .map(|v| v.val_str().to_string())
                    .unwrap_or_else(|| "unknown".to_string());

                let url = format!("http://{}:{}{}", host, port, path);
                let name = info
                    .get_fullname()
                    .split('.')
                    .next()
                    .unwrap_or("Orb Server")
                    .to_string();

                if !servers.iter().any(|s| s.url == url) {
                    servers.push(DiscoveredServer {
                        name,
                        host,
                        port,
                        url,
                        version,
                    });
                }
            }
            _ => continue,
        }
    }

    let _ = mdns.stop_browse("_orb._tcp.local.");
    let _ = mdns.shutdown();

    Ok(servers)
}

// ---- Discord Rich Presence ----

/// Managed state for the Discord RPC client.
pub struct DiscordState {
    client: Mutex<Option<discord_presence::Client>>,
}

/// Initialise (or re-initialise) the Discord RPC connection.
/// `app_id` is your Discord application's Client ID.
#[tauri::command]
fn discord_connect(
    app_id: u64,
    state: tauri::State<'_, DiscordState>,
) -> Result<(), String> {
    let mut lock = state.client.lock().map_err(|e| e.to_string())?;
    // Drop existing client first.
    *lock = None;
    let mut client = discord_presence::Client::new(app_id);
    client.start();
    *lock = Some(client);
    Ok(())
}

/// Update the Discord presence with the currently playing track.
#[tauri::command]
fn discord_update(
    title: String,
    artist: String,
    album: String,
    state: tauri::State<'_, DiscordState>,
) -> Result<(), String> {
    let mut lock = state.client.lock().map_err(|e| e.to_string())?;
    if let Some(client) = lock.as_mut() {
        let details = title;
        let status = if album.is_empty() {
            format!("by {artist}")
        } else {
            format!("by {artist} — {album}")
        };
        client
            .set_activity(|act| act.details(&details).state(&status))
            .map_err(|e| format!("discord set_activity: {e}"))?;
    }
    Ok(())
}

/// Clear the Discord presence (e.g. on pause or disconnect).
#[tauri::command]
fn discord_clear(state: tauri::State<'_, DiscordState>) -> Result<(), String> {
    let mut lock = state.client.lock().map_err(|e| e.to_string())?;
    if let Some(client) = lock.as_mut() {
        client
            .clear_activity()
            .map_err(|e| format!("discord clear_activity: {e}"))?;
    }
    Ok(())
}

/// Disconnect the Discord RPC client entirely.
#[tauri::command]
fn discord_disconnect(state: tauri::State<'_, DiscordState>) -> Result<(), String> {
    let mut lock = state.client.lock().map_err(|e| e.to_string())?;
    *lock = None;
    Ok(())
}

// ---- System Tray ----

#[cfg(desktop)]
/// IDs for tray menu items — used to identify click events and locate items for updates.
const TRAY_PREVIOUS: &str = "tray_previous";
#[cfg(desktop)]
const TRAY_PLAY_PAUSE: &str = "tray_play_pause";
#[cfg(desktop)]
const TRAY_NEXT: &str = "tray_next";
#[cfg(desktop)]
const TRAY_QUIT: &str = "tray_quit";

/// Managed state for the tray play/pause menu item so we can update its label.
#[cfg(desktop)]
pub struct TrayState {
    play_pause_item: Mutex<Option<MenuItem<tauri::Wry>>>,
}

/// Call from the frontend when playback state changes so the tray label stays in sync.
#[cfg(desktop)]
#[tauri::command]
fn set_tray_playback_state(
    playing: bool,
    state: tauri::State<'_, TrayState>,
) -> Result<(), String> {
    let lock = state.play_pause_item.lock().map_err(|e: std::sync::PoisonError<_>| e.to_string())?;
    if let Some(item) = lock.as_ref() {
        let label = if playing { "⏸  Pause" } else { "▶  Play" };
        item.set_text(label).map_err(|e: tauri::Error| e.to_string())?;
    }
    Ok(())
}

/// Mobile stub — tray is not available on mobile.
#[cfg(mobile)]
#[tauri::command]
fn set_tray_playback_state(_playing: bool) -> Result<(), String> {
    Ok(())
}

/// Build and register the system tray with media control menu items.
#[cfg(desktop)]
fn setup_tray(app: &tauri::App) -> tauri::Result<()> {
    let previous = MenuItem::with_id(app, TRAY_PREVIOUS, "⏮  Previous", true, None::<&str>)?;
    let play_pause = MenuItem::with_id(app, TRAY_PLAY_PAUSE, "▶  Play", true, None::<&str>)?;
    let next = MenuItem::with_id(app, TRAY_NEXT, "⏭  Next", true, None::<&str>)?;
    let quit = MenuItem::with_id(app, TRAY_QUIT, "Quit Orb", true, None::<&str>)?;

    let menu = Menu::with_items(app, &[&previous, &play_pause, &next, &quit])?;

    // Store the play/pause item so `set_tray_playback_state` can update it.
    app.state::<TrayState>()
        .play_pause_item
        .lock()
        .unwrap()
        .replace(play_pause);

    TrayIconBuilder::new()
        .icon(app.default_window_icon().unwrap().clone())
        .menu(&menu)
        .show_menu_on_left_click(false)
        .on_menu_event(|app: &tauri::AppHandle, event: tauri::menu::MenuEvent| match event.id().as_ref() {
            TRAY_PREVIOUS => {
                let _ = app.emit("tray-previous", ());
            }
            TRAY_PLAY_PAUSE => {
                let _ = app.emit("tray-play-pause", ());
            }
            TRAY_NEXT => {
                let _ = app.emit("tray-next", ());
            }
            TRAY_QUIT => {
                app.exit(0);
            }
            _ => {}
        })
        .on_tray_icon_event(|tray_icon: &tauri::tray::TrayIcon, event: tauri::tray::TrayIconEvent| {
            use tauri::tray::TrayIconEvent;
            if let TrayIconEvent::Click { .. } = event {
                let app = tray_icon.app_handle();
                if let Some(window) = app.get_webview_window("main") {
                    let _ = window.show();
                    let _ = window.set_focus();
                }

            }
        })
        .build(app)?;

    Ok(())
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    #[allow(unused_mut)]
    let mut builder = tauri::Builder::default()
        .manage(DiscordState {
            client: Mutex::new(None),
        });

    #[cfg(desktop)]
    let builder = builder.manage(TrayState {
        play_pause_item: Mutex::new(None),
    });

    builder
        .setup(|app| {
            // Only set up the tray on desktop targets.
            #[cfg(not(any(target_os = "android", target_os = "ios")))]
            setup_tray(app)?;
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
