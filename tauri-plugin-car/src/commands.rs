use tauri::Runtime;
use crate::models::{Track, MediaItem, PlaybackStatePayload};

#[cfg(target_os = "android")]
use crate::CarPlugin;

#[tauri::command]
#[allow(unused_variables)]
pub async fn set_now_playing<R: Runtime>(
    app: tauri::AppHandle<R>,
    track: Track,
) -> Result<(), String> {
    #[cfg(target_os = "android")]
    {
        use tauri::Manager;
        let handle = app.state::<CarPlugin<R>>();
        handle.set_now_playing(track)?;
    }
    Ok(())
}

#[tauri::command]
#[allow(unused_variables)]
pub async fn set_media_root<R: Runtime>(
    app: tauri::AppHandle<R>,
    items: Vec<MediaItem>,
) -> Result<(), String> {
    #[cfg(target_os = "android")]
    {
        use tauri::Manager;
        let handle = app.state::<CarPlugin<R>>();
        handle.set_media_root(items)?;
    }
    Ok(())
}

#[tauri::command]
#[allow(unused_variables)]
pub async fn set_playback_state<R: Runtime>(
    app: tauri::AppHandle<R>,
    playing: bool,
    position_ms: u64,
) -> Result<(), String> {
    #[cfg(target_os = "android")]
    {
        use tauri::Manager;
        let handle = app.state::<CarPlugin<R>>();
        handle.set_playback_state(PlaybackStatePayload { playing, position_ms })?;
    }
    Ok(())
}

#[tauri::command]
#[allow(unused_variables)]
pub async fn on_car_connected<R: Runtime>(
    app: tauri::AppHandle<R>,
) -> Result<(), String> {
    Ok(())
}
