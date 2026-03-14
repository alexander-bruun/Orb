#[cfg(desktop)]
mod desktop;

#[cfg(target_os = "android")]
mod android_bridge;

// ─── Android Media Commands ─────────────────────────────────────────────────

#[cfg(target_os = "android")]
#[tauri::command]
fn play_music(url: String, title: Option<String>, artist: Option<String>, cover_url: Option<String>) -> Result<(), String> {
    android_bridge::play(url, title, artist, cover_url)
}

#[cfg(target_os = "android")]
#[tauri::command]
fn pause_music() -> Result<(), String> {
    android_bridge::pause()
}

#[cfg(target_os = "android")]
#[tauri::command]
fn resume_music() -> Result<(), String> {
    android_bridge::resume()
}

#[cfg(target_os = "android")]
#[tauri::command]
fn seek_music(position_ms: i64) -> Result<(), String> {
    android_bridge::seek(position_ms)
}

#[cfg(target_os = "android")]
#[tauri::command]
fn get_position_music() -> Result<i64, String> {
    android_bridge::get_position()
}

#[cfg(target_os = "android")]
#[tauri::command]
fn get_duration_music() -> Result<i64, String> {
    android_bridge::get_duration()
}

#[cfg(target_os = "android")]
#[tauri::command]
fn get_is_playing() -> Result<bool, String> {
    android_bridge::get_is_playing()
}

#[cfg(target_os = "android")]
#[tauri::command]
fn set_shuffle_state(shuffled: bool) -> Result<(), String> {
    android_bridge::set_shuffle_state(shuffled)
}

#[cfg(target_os = "android")]
#[tauri::command]
fn set_favorite_state(favorited: bool) -> Result<(), String> {
    android_bridge::set_favorite_state(favorited)
}

#[cfg(target_os = "android")]
#[tauri::command]
fn set_api_credentials(base_url: String, token: String) -> Result<(), String> {
    android_bridge::set_api_credentials(base_url, token)
}

#[cfg(target_os = "android")]
#[tauri::command]
fn sync_downloads(metadata_json: String) -> Result<(), String> {
    android_bridge::sync_downloads(metadata_json)
}

#[cfg(target_os = "android")]
#[tauri::command]
fn save_offline_file(track_id: String, data: Vec<u8>) -> Result<String, String> {
    android_bridge::save_offline_file(track_id, data)
}

#[cfg(target_os = "android")]
#[tauri::command]
fn delete_offline_file(track_id: String) -> Result<(), String> {
    android_bridge::delete_offline_file(track_id)
}

#[cfg(target_os = "android")]
#[tauri::command]
fn save_cover_art(album_id: String, data: Vec<u8>) -> Result<(), String> {
    android_bridge::save_cover_art(album_id, data)
}

#[cfg(target_os = "android")]
#[tauri::command]
fn delete_cover_art(album_id: String) -> Result<(), String> {
    android_bridge::delete_cover_art(album_id)
}

#[cfg(target_os = "android")]
#[tauri::command]
fn set_volume(volume: f32) -> Result<(), String> {
    android_bridge::set_volume(volume)
}

#[cfg(target_os = "android")]
#[tauri::command]
fn get_volume() -> Result<f32, String> {
    android_bridge::get_volume()
}

#[cfg(target_os = "android")]
#[tauri::command]
fn get_device_id() -> Result<String, String> {
    android_bridge::get_device_id()
}

#[cfg(target_os = "android")]
#[tauri::command]
fn set_eq_bands(enabled: bool, bands_json: String) -> Result<(), String> {
    android_bridge::set_eq_bands(enabled, bands_json)
}

#[cfg(target_os = "android")]
#[tauri::command]
fn set_crossfade_settings(enabled: bool, secs: f32) -> Result<(), String> {
    android_bridge::set_crossfade_settings(enabled, secs)
}

#[cfg(target_os = "android")]
#[tauri::command]
fn set_gapless_enabled(enabled: bool) -> Result<(), String> {
    android_bridge::set_gapless_enabled(enabled)
}

// ─── Entry Point ─────────────────────────────────────────────────────────────

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let builder = tauri::Builder::default();

    // Desktop: tray, mDNS, Discord RPC — includes its own invoke_handler.
    #[cfg(desktop)]
    let builder = desktop::setup(builder);

    // Mobile: native media playback commands (ExoPlayer on Android).
    #[cfg(target_os = "android")]
    let builder = builder
        .invoke_handler(tauri::generate_handler![
            play_music,
            pause_music,
            resume_music,
            seek_music,
            get_position_music,
            get_duration_music,
            get_is_playing,
            set_shuffle_state,
            set_favorite_state,
            set_api_credentials,
            sync_downloads,
            save_offline_file,
            delete_offline_file,
            save_cover_art,
            delete_cover_art,
            set_volume,
            get_volume,
            get_device_id,
            set_eq_bands,
            set_crossfade_settings,
            set_gapless_enabled,
        ])
        .setup(|app| {
            let _ = android_bridge::APP_HANDLE.set(app.handle().clone());
            Ok(())
        });

    builder
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
