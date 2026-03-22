#[cfg(target_os = "android")]
use jni::objects::{Global, JClass, JObject, JValue};
#[cfg(target_os = "android")]
use jni::Env;
#[cfg(target_os = "android")]
use jni::EnvUnowned;
#[cfg(target_os = "android")]
use jni::JavaVM;
#[cfg(target_os = "android")]
use jni::{jni_sig, jni_str};
#[cfg(target_os = "android")]
use jni::sys;
#[cfg(target_os = "android")]
use jni::sys::jint;
#[cfg(target_os = "android")]
use jni::sys::jlong;
#[cfg(target_os = "android")]
use once_cell::sync::OnceCell;
#[cfg(target_os = "android")]
use std::os::raw::c_void;

#[cfg(target_os = "android")]
static JVM: OnceCell<JavaVM> = OnceCell::new();

#[cfg(target_os = "android")]
static MEDIA_CLASSLOADER: OnceCell<Global<JObject<'static>>> = OnceCell::new();

#[cfg(target_os = "android")]
pub(crate) static APP_HANDLE: OnceCell<tauri::AppHandle> = OnceCell::new();

/// Stable hardware identifier (Settings.Secure.ANDROID_ID), set once from nativeInit.
#[cfg(target_os = "android")]
static DEVICE_ID: OnceCell<String> = OnceCell::new();

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn JNI_OnLoad(vm: *mut sys::JavaVM, _reserved: *mut c_void) -> jint {
    let jvm = unsafe { JavaVM::from_raw(vm) };
    let _ = JVM.set(jvm);
    sys::JNI_VERSION_1_6
}

#[cfg(target_os = "android")]
fn get_jvm() -> &'static JavaVM {
    JVM.get().expect("JavaVM not initialized: JNI_OnLoad not called")
}

#[cfg(target_os = "android")]
fn get_companion_class<'a>(env: &mut Env<'a>) -> Result<JClass<'a>, jni::errors::Error> {
    if let Some(loader) = MEDIA_CLASSLOADER.get() {
        let name = env.new_string("com/orb/app/MediaService")?;
        let cls_obj = env.call_method(
            loader.as_obj(),
            jni_str!("loadClass"),
            jni_sig!("(Ljava/lang/String;)Ljava/lang/Class;"),
            &[JValue::Object(&name)],
        )?.l()?;
        Ok(unsafe { JClass::from_raw(env, cls_obj.as_raw()) })
    } else {
        env.find_class(jni_str!("com/orb/app/MediaService"))
    }
}

// ── JNI boilerplate helpers ──────────────────────────────────────────────────

/// Attach to the JVM, run the closure with a mutable Env, and map errors.
#[cfg(target_os = "android")]
fn with_jni<T>(f: impl FnOnce(&mut Env) -> Result<T, jni::errors::Error>) -> Result<T, String> {
    let jvm = get_jvm();
    jvm.attach_current_thread(f).map_err(|e| e.to_string())
}

// ── Emit Tauri events to the frontend ────────────────────────────────────────

#[cfg(target_os = "android")]
fn emit_to_frontend(event: &str) {
    use tauri::Emitter;
    if let Some(handle) = APP_HANDLE.get() {
        let _ = handle.emit(event, ());
    }
}

// ── JNI callbacks: called from Kotlin MediaService notification actions ──────

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn Java_com_orb_app_MediaService_nativeOnNext(
    _env: EnvUnowned,
    _class: JClass,
) {
    emit_to_frontend("native-next");
}

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn Java_com_orb_app_MediaService_nativeOnPrevious(
    _env: EnvUnowned,
    _class: JClass,
) {
    emit_to_frontend("native-previous");
}

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn Java_com_orb_app_MediaService_nativeOnShuffleToggle(
    _env: EnvUnowned,
    _class: JClass,
) {
    emit_to_frontend("native-shuffle-toggle");
}

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn Java_com_orb_app_MediaService_nativeOnFavoriteToggle(
    _env: EnvUnowned,
    _class: JClass,
) {
    emit_to_frontend("native-favorite-toggle");
}

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn Java_com_orb_app_MediaService_nativeOnPause(
    _env: EnvUnowned,
    _class: JClass,
) {
    emit_to_frontend("native-pause");
}

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn Java_com_orb_app_MediaService_nativeOnPlay(
    _env: EnvUnowned,
    _class: JClass,
) {
    emit_to_frontend("native-play");
}

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn Java_com_orb_app_MediaService_nativeOnVolumeChange(
    _env: EnvUnowned,
    _class: JClass,
    volume: jni::sys::jfloat,
) {
    use tauri::Emitter;
    if let Some(handle) = APP_HANDLE.get() {
        let _ = handle.emit("native-volume-change", volume as f64);
    }
}

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn Java_com_orb_app_MediaService_nativeOnDownloadProgress(
    _env: EnvUnowned,
    _class: JClass,
    track_id: jni::objects::JString,
    progress: jint,
    total_bytes: jlong,
) {
    use tauri::Emitter;
    if let Some(handle) = APP_HANDLE.get() {
        let id_str = track_id.to_string();
        let _ = handle.emit("download-progress", (id_str, progress as i32, total_bytes as i64));
    }
}

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn Java_com_orb_app_MediaService_nativeOnABSkipBack15(
    _env: EnvUnowned,
    _class: JClass,
) {
    emit_to_frontend("native-ab-skip-back-15");
}

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn Java_com_orb_app_MediaService_nativeOnABSkipForward15(
    _env: EnvUnowned,
    _class: JClass,
) {
    emit_to_frontend("native-ab-skip-forward-15");
}

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn Java_com_orb_app_MediaService_nativeOnABSpeedCycle(
    _env: EnvUnowned,
    _class: JClass,
) {
    emit_to_frontend("native-ab-speed-cycle");
}

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn Java_com_orb_app_MediaService_nativeOnABChapterStart(
    _env: EnvUnowned,
    _class: JClass,
) {
    emit_to_frontend("native-ab-chapter-start");
}

// ── Playback commands: called from Rust Tauri commands ───────────────────────

#[cfg(target_os = "android")]
pub fn play(url: String, title: Option<String>, artist: Option<String>, cover_url: Option<String>) -> Result<(), String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        let url_jstring = env.new_string(&url)?;
        let null = JObject::null();

        let title_jstring;
        let title_val = if let Some(ref t) = title {
            title_jstring = env.new_string(t)?;
            JValue::Object(&title_jstring)
        } else {
            JValue::Object(&null)
        };

        let artist_jstring;
        let artist_val = if let Some(ref a) = artist {
            artist_jstring = env.new_string(a)?;
            JValue::Object(&artist_jstring)
        } else {
            JValue::Object(&null)
        };

        let cover_jstring;
        let cover_val = if let Some(ref c) = cover_url {
            cover_jstring = env.new_string(c)?;
            JValue::Object(&cover_jstring)
        } else {
            JValue::Object(&null)
        };

        env.call_static_method(
            cls, jni_str!("playTrack"),
            jni_sig!("(Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;)V"),
            &[JValue::Object(&url_jstring), title_val, artist_val, cover_val],
        )?;
        Ok(())
    })
}

#[cfg(target_os = "android")]
pub fn pause() -> Result<(), String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        env.call_static_method(cls, jni_str!("pauseTrack"), jni_sig!("()V"), &[])?;
        Ok(())
    })
}

#[cfg(target_os = "android")]
pub fn resume() -> Result<(), String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        env.call_static_method(cls, jni_str!("resumeTrack"), jni_sig!("()V"), &[])?;
        Ok(())
    })
}

#[cfg(target_os = "android")]
pub fn seek(position_ms: i64) -> Result<(), String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        env.call_static_method(cls, jni_str!("seekTo"), jni_sig!("(J)V"), &[JValue::Long(position_ms)])?;
        Ok(())
    })
}

#[cfg(target_os = "android")]
pub fn get_position() -> Result<i64, String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        Ok(env.call_static_method(cls, jni_str!("getPosition"), jni_sig!("()J"), &[])?.j()? as i64)
    })
}

#[cfg(target_os = "android")]
pub fn get_duration() -> Result<i64, String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        Ok(env.call_static_method(cls, jni_str!("getDuration"), jni_sig!("()J"), &[])?.j()? as i64)
    })
}

#[cfg(target_os = "android")]
pub fn get_is_playing() -> Result<bool, String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        Ok(env.call_static_method(cls, jni_str!("getIsPlaying"), jni_sig!("()Z"), &[])?.z()?)
    })
}

#[cfg(target_os = "android")]
pub fn set_shuffle_state(shuffled: bool) -> Result<(), String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        env.call_static_method(cls, jni_str!("setShuffleState"), jni_sig!("(Z)V"), &[JValue::Bool(shuffled)])?;
        Ok(())
    })
}

#[cfg(target_os = "android")]
pub fn set_favorite_state(favorited: bool) -> Result<(), String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        env.call_static_method(cls, jni_str!("setFavoriteState"), jni_sig!("(Z)V"), &[JValue::Bool(favorited)])?;
        Ok(())
    })
}

#[cfg(target_os = "android")]
pub fn set_audiobook_mode(is_audiobook: bool) -> Result<(), String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        env.call_static_method(cls, jni_str!("setAudiobookMode"), jni_sig!("(Z)V"), &[JValue::Bool(is_audiobook)])?;
        Ok(())
    })
}

#[cfg(target_os = "android")]
pub fn set_playback_speed(speed: f32) -> Result<(), String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        env.call_static_method(cls, jni_str!("setPlaybackSpeed"), jni_sig!("(F)V"), &[JValue::Float(speed)])?;
        Ok(())
    })
}

#[cfg(target_os = "android")]
pub fn set_api_credentials(base_url: String, token: String) -> Result<(), String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        let base_url_jstring = env.new_string(&base_url)?;
        let token_jstring = env.new_string(&token)?;
        env.call_static_method(
            cls, jni_str!("setApiCredentials"),
            jni_sig!("(Ljava/lang/String;Ljava/lang/String;)V"),
            &[JValue::Object(&base_url_jstring), JValue::Object(&token_jstring)],
        )?;
        Ok(())
    })
}

#[cfg(target_os = "android")]
pub fn sync_downloads(metadata_json: String) -> Result<(), String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        let json_jstring = env.new_string(&metadata_json)?;
        env.call_static_method(
            cls, jni_str!("syncDownloads"),
            jni_sig!("(Ljava/lang/String;)V"),
            &[JValue::Object(&json_jstring)],
        )?;
        Ok(())
    })
}

#[cfg(target_os = "android")]
pub fn save_offline_file(track_id: String, data: Vec<u8>) -> Result<String, String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        let id_jstring = env.new_string(&track_id)?;
        let byte_array = env.byte_array_from_slice(&data)?;
        let result = env.call_static_method(
            cls, jni_str!("saveOfflineFile"),
            jni_sig!("(Ljava/lang/String;[B)Ljava/lang/String;"),
            &[JValue::Object(&id_jstring), JValue::Object(&byte_array)],
        )?.l()?;
        let jstr = unsafe { jni::objects::JString::from_raw(env, result.as_raw()) };
        Ok(jstr.to_string())
    })
}

#[cfg(target_os = "android")]
pub fn download_track_native(
    track_id: String,
    url: String,
    token: Option<String>,
) -> Result<String, String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        let id_jstring = env.new_string(&track_id)?;
        let url_jstring = env.new_string(&url)?;
        let null_token = JObject::null();
        let token_jstring;
        let token_obj = if let Some(ref t) = token {
            token_jstring = env.new_string(t)?;
            JObject::from(token_jstring)
        } else {
            null_token
        };
        let result = env.call_static_method(
            cls, jni_str!("downloadTrackNative"),
            jni_sig!("(Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String;"),
            &[JValue::Object(&id_jstring), JValue::Object(&url_jstring), JValue::Object(&token_obj)],
        )?.l()?;
        let jstr = unsafe { jni::objects::JString::from_raw(env, result.as_raw()) };
        Ok(jstr.to_string())
    })
}

#[cfg(target_os = "android")]
pub fn save_cover_art(album_id: String, data: Vec<u8>) -> Result<(), String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        let id_jstring = env.new_string(&album_id)?;
        let byte_array = env.byte_array_from_slice(&data)?;
        env.call_static_method(
            cls, jni_str!("saveCoverArt"),
            jni_sig!("(Ljava/lang/String;[B)V"),
            &[JValue::Object(&id_jstring), JValue::Object(&byte_array)],
        )?;
        Ok(())
    })
}

#[cfg(target_os = "android")]
pub fn delete_cover_art(album_id: String) -> Result<(), String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        let id_jstring = env.new_string(&album_id)?;
        env.call_static_method(
            cls, jni_str!("deleteCoverArt"),
            jni_sig!("(Ljava/lang/String;)V"),
            &[JValue::Object(&id_jstring)],
        )?;
        Ok(())
    })
}

#[cfg(target_os = "android")]
pub fn get_offline_file_path(track_id: String) -> Result<Option<String>, String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        let id_jstring = env.new_string(&track_id)?;
        let result = env.call_static_method(
            cls, jni_str!("getOfflineFilePath"),
            jni_sig!("(Ljava/lang/String;)Ljava/lang/String;"),
            &[JValue::Object(&id_jstring)],
        )?.l()?;
        if result.is_null() {
            Ok(None)
        } else {
            let jstr = unsafe { jni::objects::JString::from_raw(env, result.as_raw()) };
            Ok(Some(jstr.to_string()))
        }
    })
}

#[cfg(target_os = "android")]
pub fn delete_offline_file(track_id: String) -> Result<(), String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        let id_jstring = env.new_string(&track_id)?;
        env.call_static_method(
            cls, jni_str!("deleteOfflineFile"),
            jni_sig!("(Ljava/lang/String;)V"),
            &[JValue::Object(&id_jstring)],
        )?;
        Ok(())
    })
}

#[cfg(target_os = "android")]
pub fn set_volume(volume: f32) -> Result<(), String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        env.call_static_method(cls, jni_str!("setVolume"), jni_sig!("(F)V"), &[JValue::Float(volume)])?;
        Ok(())
    })
}

#[cfg(target_os = "android")]
pub fn get_volume() -> Result<f32, String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        Ok(env.call_static_method(cls, jni_str!("getVolume"), jni_sig!("()F"), &[])?.f()?)
    })
}

/// Apply EQ bands to the hardware Android Equalizer.
/// `bands_json` is a JSON array of {frequency: Hz, gain: dB} objects.
/// Pass `enabled = false` to bypass the equalizer without clearing the bands.
#[cfg(target_os = "android")]
pub fn set_eq_bands(enabled: bool, bands_json: String) -> Result<(), String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        let json_jstring = env.new_string(&bands_json)?;
        env.call_static_method(
            cls, jni_str!("setEQBands"),
            jni_sig!("(ZLjava/lang/String;)V"),
            &[JValue::Bool(enabled), JValue::Object(&json_jstring)],
        )?;
        Ok(())
    })
}

/// Configure crossfade. When enabled, ExoPlayer fires nativeOnNext() [secs]
/// seconds before the track ends; handlePlay() then cross-fades the volumes.
#[cfg(target_os = "android")]
pub fn set_crossfade_settings(enabled: bool, secs: f32) -> Result<(), String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        env.call_static_method(
            cls, jni_str!("setCrossfadeSettings"),
            jni_sig!("(ZF)V"),
            &[JValue::Bool(enabled), JValue::Float(secs)],
        )?;
        Ok(())
    })
}

/// Enable or disable gapless playback (reserved for future pre-buffering logic).
#[cfg(target_os = "android")]
pub fn set_gapless_enabled(enabled: bool) -> Result<(), String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        env.call_static_method(cls, jni_str!("setGaplessEnabled"), jni_sig!("(Z)V"), &[JValue::Bool(enabled)])?;
        Ok(())
    })
}

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn Java_com_orb_app_MainActivity_nativeInit(
    mut env: EnvUnowned,
    _class: JClass,
    class_loader: JObject,
    device_id: jni::objects::JString,
) {
    let _ = env.with_env(|env| {
        if let Ok(global) = env.new_global_ref(&class_loader) {
            let _ = MEDIA_CLASSLOADER.set(global);
        }
        let id_str = device_id.to_string();
        if !id_str.is_empty() {
            let _ = DEVICE_ID.set(id_str);
        }
        Ok::<(), jni::errors::Error>(())
    });
}

#[cfg(target_os = "android")]
pub fn get_device_id() -> Result<String, String> {
    Ok(DEVICE_ID.get().cloned().unwrap_or_default())
}

#[cfg(target_os = "android")]
pub fn open_bluetooth_settings() -> Result<(), String> {
    with_jni(|env| {
        let cls = get_companion_class(env)?;
        env.call_static_method(cls, jni_str!("openBluetoothSettings"), jni_sig!("()V"), &[])?;
        Ok(())
    })
}
