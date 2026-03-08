use serde::{Deserialize, Serialize};
use tauri::{
    plugin::{Builder, TauriPlugin},
    Runtime,
};

#[derive(Debug, Serialize, Deserialize)]
#[allow(dead_code)]
struct LoadTrackPayload {
    url: String,
    title: String,
    artist: String,
    album: Option<String>,
    artwork: Option<String>,
}

#[derive(Debug, Serialize, Deserialize)]
#[allow(dead_code)]
struct SeekPayload {
    position_ms: u64,
}

#[cfg(target_os = "android")]
mod mobile {
    use super::*;
    use tauri::plugin::PluginHandle;

    pub struct MediaSession<R: Runtime>(pub PluginHandle<R>);

    impl<R: Runtime> MediaSession<R> {
        pub fn initialize_player(&self) -> Result<(), String> {
            self.0
                .run_mobile_plugin::<()>("initialize_player", ())
                .map_err(|e| e.to_string())
        }

        pub fn load_track(&self, payload: LoadTrackPayload) -> Result<(), String> {
            self.0
                .run_mobile_plugin::<()>("load_track", payload)
                .map_err(|e| e.to_string())
        }

        pub fn play(&self) -> Result<(), String> {
            self.0
                .run_mobile_plugin::<()>("play", ())
                .map_err(|e| e.to_string())
        }

        pub fn pause(&self) -> Result<(), String> {
            self.0
                .run_mobile_plugin::<()>("pause", ())
                .map_err(|e| e.to_string())
        }

        pub fn next_track(&self) -> Result<(), String> {
            self.0
                .run_mobile_plugin::<()>("next_track", ())
                .map_err(|e| e.to_string())
        }

        pub fn previous_track(&self) -> Result<(), String> {
            self.0
                .run_mobile_plugin::<()>("previous_track", ())
                .map_err(|e| e.to_string())
        }

        pub fn seek(&self, payload: SeekPayload) -> Result<(), String> {
            self.0
                .run_mobile_plugin::<()>("seek", payload)
                .map_err(|e| e.to_string())
        }

        pub fn stop(&self) -> Result<(), String> {
            self.0
                .run_mobile_plugin::<()>("stop", ())
                .map_err(|e| e.to_string())
        }

        pub fn write_log(&self, message: String) -> Result<(), String> {
            self.0
                .run_mobile_plugin::<()>("write_log", serde_json::json!({ "message": message }))
                .map_err(|e| e.to_string())
        }

        pub fn get_log(&self) -> Result<String, String> {
            let v: serde_json::Value = self.0
                .run_mobile_plugin("get_log", ())
                .map_err(|e| e.to_string())?;
            Ok(v.get("value")
                .and_then(|v| v.as_str())
                .unwrap_or("")
                .to_string())
        }
    }
}

#[cfg(target_os = "android")]
use mobile::MediaSession;

// ── Commands ─────────────────────────────────────────────────────────────────

#[tauri::command]
#[allow(unused_variables)]
async fn initialize_player<R: Runtime>(app: tauri::AppHandle<R>) -> Result<(), String> {
    #[cfg(target_os = "android")]
    {
        use tauri::Manager;
        app.state::<MediaSession<R>>().initialize_player()?;
    }
    Ok(())
}

#[tauri::command]
#[allow(unused_variables)]
async fn load_track<R: Runtime>(
    app: tauri::AppHandle<R>,
    url: String,
    title: String,
    artist: String,
    album: Option<String>,
    artwork: Option<String>,
) -> Result<(), String> {
    #[cfg(target_os = "android")]
    {
        use tauri::Manager;
        app.state::<MediaSession<R>>().load_track(LoadTrackPayload {
            url,
            title,
            artist,
            album,
            artwork,
        })?;
    }
    Ok(())
}

#[tauri::command]
#[allow(unused_variables)]
async fn play<R: Runtime>(app: tauri::AppHandle<R>) -> Result<(), String> {
    #[cfg(target_os = "android")]
    {
        use tauri::Manager;
        app.state::<MediaSession<R>>().play()?;
    }
    Ok(())
}

#[tauri::command]
#[allow(unused_variables)]
async fn pause<R: Runtime>(app: tauri::AppHandle<R>) -> Result<(), String> {
    #[cfg(target_os = "android")]
    {
        use tauri::Manager;
        app.state::<MediaSession<R>>().pause()?;
    }
    Ok(())
}

#[tauri::command]
#[allow(unused_variables)]
async fn next_track<R: Runtime>(app: tauri::AppHandle<R>) -> Result<(), String> {
    #[cfg(target_os = "android")]
    {
        use tauri::Manager;
        app.state::<MediaSession<R>>().next_track()?;
    }
    Ok(())
}

#[tauri::command]
#[allow(unused_variables)]
async fn previous_track<R: Runtime>(app: tauri::AppHandle<R>) -> Result<(), String> {
    #[cfg(target_os = "android")]
    {
        use tauri::Manager;
        app.state::<MediaSession<R>>().previous_track()?;
    }
    Ok(())
}

#[tauri::command]
#[allow(unused_variables)]
async fn seek<R: Runtime>(app: tauri::AppHandle<R>, position_ms: u64) -> Result<(), String> {
    #[cfg(target_os = "android")]
    {
        use tauri::Manager;
        app.state::<MediaSession<R>>().seek(SeekPayload { position_ms })?;
    }
    Ok(())
}

#[tauri::command]
#[allow(unused_variables)]
async fn stop<R: Runtime>(app: tauri::AppHandle<R>) -> Result<(), String> {
    #[cfg(target_os = "android")]
    {
        use tauri::Manager;
        app.state::<MediaSession<R>>().stop()?;
    }
    Ok(())
}

#[tauri::command]
#[allow(unused_variables)]
async fn get_log<R: Runtime>(app: tauri::AppHandle<R>) -> Result<String, String> {
    #[cfg(target_os = "android")]
    {
        use tauri::Manager;
        return app.state::<MediaSession<R>>().get_log();
    }
    #[cfg(not(target_os = "android"))]
    Ok(String::new())
}

#[tauri::command]
#[allow(unused_variables)]
async fn write_log<R: Runtime>(app: tauri::AppHandle<R>, message: String) -> Result<(), String> {
    #[cfg(target_os = "android")]
    {
        use tauri::Manager;
        app.state::<MediaSession<R>>().write_log(message)?;
    }
    Ok(())
}

// ── Plugin Init ──────────────────────────────────────────────────────────────

pub fn init<R: Runtime>() -> TauriPlugin<R> {
    Builder::new("mediasession")
        .invoke_handler(tauri::generate_handler![
            initialize_player,
            load_track,
            play,
            pause,
            next_track,
            previous_track,
            seek,
            stop,
            get_log,
            write_log,
        ])
        .setup(|app, _api| {
            #[cfg(target_os = "android")]
            {
                use tauri::Manager;
                let handle = _api
                    .register_android_plugin("com.plugin.mediasession", "MediaSessionPlugin")?;
                app.manage(MediaSession(handle));
            }
            let _ = app;
            Ok(())
        })
        .build()
}
