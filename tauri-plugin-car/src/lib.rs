use tauri::{
    plugin::{Builder, TauriPlugin},
    Runtime,
};

pub mod commands;
pub mod models;

#[cfg(target_os = "android")]
mod mobile {
    use super::*;
    use crate::models::{Track, MediaItem, PlaybackStatePayload};
    use tauri::plugin::PluginHandle;

    pub struct CarPlugin<R: Runtime>(pub PluginHandle<R>);

    impl<R: Runtime> CarPlugin<R> {
        pub fn set_now_playing(&self, track: Track) -> Result<(), String> {
            self.0.run_mobile_plugin("set_now_playing", track)
                .map_err(|e| e.to_string())
        }

        pub fn set_media_root(&self, items: Vec<MediaItem>) -> Result<(), String> {
            self.0.run_mobile_plugin("set_media_root", serde_json::json!({ "items": items }))
                .map_err(|e| e.to_string())
        }

        pub fn set_playback_state(&self, payload: PlaybackStatePayload) -> Result<(), String> {
            self.0.run_mobile_plugin("set_playback_state", payload)
                .map_err(|e| e.to_string())
        }
    }
}

#[cfg(target_os = "android")]
pub(crate) use mobile::CarPlugin;

pub fn init<R: Runtime>() -> TauriPlugin<R> {
    Builder::new("car")
        .invoke_handler(tauri::generate_handler![
            commands::set_now_playing,
            commands::set_media_root,
            commands::set_playback_state,
            commands::on_car_connected,
        ])
        .setup(|app, _api| {
            #[cfg(target_os = "android")]
            {
                use tauri::Manager;
                let handle = _api.register_android_plugin("com.plugin.car", "CarPlugin")?;
                app.manage(CarPlugin(handle));
            }
            let _ = app;
            Ok(())
        })
        .build()
}
