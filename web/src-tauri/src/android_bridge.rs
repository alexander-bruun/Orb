#[cfg(target_os = "android")]
use jni::objects::{JObject, JValue};
#[cfg(target_os = "android")]
use jni::objects::{GlobalRef, JClass};
#[cfg(target_os = "android")]
use jni::JNIEnv;
#[cfg(target_os = "android")]
use jni::JavaVM;
#[cfg(target_os = "android")]
use jni::sys;
#[cfg(target_os = "android")]
use jni::sys::jint;
#[cfg(target_os = "android")]
use once_cell::sync::OnceCell;
#[cfg(target_os = "android")]
use std::os::raw::c_void;

#[cfg(target_os = "android")]
static JVM: OnceCell<JavaVM> = OnceCell::new();

#[cfg(target_os = "android")]
static MEDIA_CLASSLOADER: OnceCell<GlobalRef> = OnceCell::new();

#[cfg(target_os = "android")]
pub(crate) static APP_HANDLE: OnceCell<tauri::AppHandle> = OnceCell::new();

/// Stable hardware identifier (Settings.Secure.ANDROID_ID), set once from nativeInit.
#[cfg(target_os = "android")]
static DEVICE_ID: OnceCell<String> = OnceCell::new();

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn JNI_OnLoad(vm: *mut sys::JavaVM, _reserved: *mut c_void) -> jint {
    let jvm = unsafe { JavaVM::from_raw(vm).expect("failed to create JavaVM from raw") };
    let _ = JVM.set(jvm);
    sys::JNI_VERSION_1_6
}

#[cfg(target_os = "android")]
fn get_jvm() -> &'static JavaVM {
    JVM.get().expect("JavaVM not initialized: JNI_OnLoad not called")
}

#[cfg(target_os = "android")]
const COMPANION_CLASS: &str = "com/orb/app/MediaService";

#[cfg(target_os = "android")]
fn get_companion_class<'a>(env: &mut JNIEnv<'a>) -> Result<JClass<'a>, jni::errors::Error> {
    if let Some(loader) = MEDIA_CLASSLOADER.get() {
        let name = env.new_string(COMPANION_CLASS)?;
        let cls_obj = env.call_method(
            loader.as_obj(),
            "loadClass",
            "(Ljava/lang/String;)Ljava/lang/Class;",
            &[JValue::Object(&name)],
        )?.l()?;
        Ok(JClass::from(cls_obj))
    } else {
        env.find_class(COMPANION_CLASS)
    }
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
    _env: JNIEnv,
    _class: JClass,
) {
    emit_to_frontend("native-next");
}

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn Java_com_orb_app_MediaService_nativeOnPrevious(
    _env: JNIEnv,
    _class: JClass,
) {
    emit_to_frontend("native-previous");
}

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn Java_com_orb_app_MediaService_nativeOnShuffleToggle(
    _env: JNIEnv,
    _class: JClass,
) {
    emit_to_frontend("native-shuffle-toggle");
}

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn Java_com_orb_app_MediaService_nativeOnFavoriteToggle(
    _env: JNIEnv,
    _class: JClass,
) {
    emit_to_frontend("native-favorite-toggle");
}

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn Java_com_orb_app_MediaService_nativeOnVolumeChange(
    _env: JNIEnv,
    _class: JClass,
    volume: jni::sys::jfloat,
) {
    use tauri::Emitter;
    if let Some(handle) = APP_HANDLE.get() {
        let _ = handle.emit("native-volume-change", volume as f64);
    }
}

// ── Playback commands: called from Rust Tauri commands ───────────────────────

#[cfg(target_os = "android")]
pub fn play(url: String, title: Option<String>, artist: Option<String>, cover_url: Option<String>) -> Result<(), String> {
    let jvm = get_jvm();
    let res: Result<(), jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;

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
            cls,
            "playTrack",
            "(Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;)V",
            &[
                JValue::Object(&url_jstring),
                title_val,
                artist_val,
                cover_val,
            ],
        )?;

        Ok(())
    })();

    res.map_err(|e| e.to_string())
}

#[cfg(target_os = "android")]
pub fn pause() -> Result<(), String> {
    let jvm = get_jvm();
    let res: Result<(), jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;

        env.call_static_method(cls, "pauseTrack", "()V", &[])?;

        Ok(())
    })();

    res.map_err(|e| e.to_string())
}

#[cfg(target_os = "android")]
pub fn resume() -> Result<(), String> {
    let jvm = get_jvm();
    let res: Result<(), jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;

        env.call_static_method(cls, "resumeTrack", "()V", &[])?;

        Ok(())
    })();

    res.map_err(|e| e.to_string())
}

#[cfg(target_os = "android")]
pub fn seek(position_ms: i64) -> Result<(), String> {
    let jvm = get_jvm();
    let res: Result<(), jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;

        env.call_static_method(
            cls,
            "seekTo",
            "(J)V",
            &[JValue::Long(position_ms)],
        )?;

        Ok(())
    })();

    res.map_err(|e| e.to_string())
}

#[cfg(target_os = "android")]
pub fn get_position() -> Result<i64, String> {
    let jvm = get_jvm();
    let res: Result<i64, jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;

        let val = env.call_static_method(cls, "getPosition", "()J", &[])?;
        Ok(val.j()? as i64)
    })();

    res.map_err(|e| e.to_string())
}

#[cfg(target_os = "android")]
pub fn get_duration() -> Result<i64, String> {
    let jvm = get_jvm();
    let res: Result<i64, jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;

        let val = env.call_static_method(cls, "getDuration", "()J", &[])?;
        Ok(val.j()? as i64)
    })();

    res.map_err(|e| e.to_string())
}

#[cfg(target_os = "android")]
pub fn get_is_playing() -> Result<bool, String> {
    let jvm = get_jvm();
    let res: Result<bool, jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;

        let val = env.call_static_method(cls, "getIsPlaying", "()Z", &[])?;
        Ok(val.z()?)
    })();

    res.map_err(|e| e.to_string())
}

#[cfg(target_os = "android")]
pub fn set_shuffle_state(shuffled: bool) -> Result<(), String> {
    let jvm = get_jvm();
    let res: Result<(), jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;

        env.call_static_method(
            cls,
            "setShuffleState",
            "(Z)V",
            &[JValue::Bool(shuffled as u8)],
        )?;

        Ok(())
    })();

    res.map_err(|e| e.to_string())
}

#[cfg(target_os = "android")]
pub fn set_favorite_state(favorited: bool) -> Result<(), String> {
    let jvm = get_jvm();
    let res: Result<(), jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;

        env.call_static_method(
            cls,
            "setFavoriteState",
            "(Z)V",
            &[JValue::Bool(favorited as u8)],
        )?;

        Ok(())
    })();

    res.map_err(|e| e.to_string())
}

#[cfg(target_os = "android")]
pub fn set_api_credentials(base_url: String, token: String) -> Result<(), String> {
    let jvm = get_jvm();
    let res: Result<(), jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;

        let base_url_jstring = env.new_string(&base_url)?;
        let token_jstring = env.new_string(&token)?;

        env.call_static_method(
            cls,
            "setApiCredentials",
            "(Ljava/lang/String;Ljava/lang/String;)V",
            &[
                JValue::Object(&base_url_jstring),
                JValue::Object(&token_jstring),
            ],
        )?;

        Ok(())
    })();

    res.map_err(|e| e.to_string())
}

#[cfg(target_os = "android")]
pub fn sync_downloads(metadata_json: String) -> Result<(), String> {
    let jvm = get_jvm();
    let res: Result<(), jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;
        let json_jstring = env.new_string(&metadata_json)?;

        env.call_static_method(
            cls,
            "syncDownloads",
            "(Ljava/lang/String;)V",
            &[JValue::Object(&json_jstring)],
        )?;

        Ok(())
    })();

    res.map_err(|e| e.to_string())
}

#[cfg(target_os = "android")]
pub fn save_offline_file(track_id: String, data: Vec<u8>) -> Result<String, String> {
    let jvm = get_jvm();
    let res: Result<String, jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;
        let id_jstring = env.new_string(&track_id)?;
        let byte_array = env.byte_array_from_slice(&data)?;

        let result = env.call_static_method(
            cls,
            "saveOfflineFile",
            "(Ljava/lang/String;[B)Ljava/lang/String;",
            &[JValue::Object(&id_jstring), JValue::Object(&byte_array)],
        )?.l()?;

        let path: String = env.get_string((&result).into())?.into();
        Ok(path)
    })();

    res.map_err(|e| e.to_string())
}

#[cfg(target_os = "android")]
pub fn save_cover_art(album_id: String, data: Vec<u8>) -> Result<(), String> {
    let jvm = get_jvm();
    let res: Result<(), jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;
        let id_jstring = env.new_string(&album_id)?;
        let byte_array = env.byte_array_from_slice(&data)?;

        env.call_static_method(
            cls,
            "saveCoverArt",
            "(Ljava/lang/String;[B)V",
            &[JValue::Object(&id_jstring), JValue::Object(&byte_array)],
        )?;

        Ok(())
    })();

    res.map_err(|e| e.to_string())
}

#[cfg(target_os = "android")]
pub fn delete_cover_art(album_id: String) -> Result<(), String> {
    let jvm = get_jvm();
    let res: Result<(), jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;
        let id_jstring = env.new_string(&album_id)?;

        env.call_static_method(
            cls,
            "deleteCoverArt",
            "(Ljava/lang/String;)V",
            &[JValue::Object(&id_jstring)],
        )?;

        Ok(())
    })();

    res.map_err(|e| e.to_string())
}

#[cfg(target_os = "android")]
pub fn delete_offline_file(track_id: String) -> Result<(), String> {
    let jvm = get_jvm();
    let res: Result<(), jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;
        let id_jstring = env.new_string(&track_id)?;

        env.call_static_method(
            cls,
            "deleteOfflineFile",
            "(Ljava/lang/String;)V",
            &[JValue::Object(&id_jstring)],
        )?;

        Ok(())
    })();

    res.map_err(|e| e.to_string())
}

#[cfg(target_os = "android")]
pub fn set_volume(volume: f32) -> Result<(), String> {
    let jvm = get_jvm();
    let res: Result<(), jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;

        env.call_static_method(
            cls,
            "setVolume",
            "(F)V",
            &[JValue::Float(volume)],
        )?;

        Ok(())
    })();

    res.map_err(|e| e.to_string())
}

#[cfg(target_os = "android")]
pub fn get_volume() -> Result<f32, String> {
    let jvm = get_jvm();
    let res: Result<f32, jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;

        let val = env.call_static_method(cls, "getVolume", "()F", &[])?;
        Ok(val.f()?)
    })();

    res.map_err(|e| e.to_string())
}

/// Apply EQ bands to the hardware Android Equalizer.
/// `bands_json` is a JSON array of {frequency: Hz, gain: dB} objects.
/// Pass `enabled = false` to bypass the equalizer without clearing the bands.
#[cfg(target_os = "android")]
pub fn set_eq_bands(enabled: bool, bands_json: String) -> Result<(), String> {
    let jvm = get_jvm();
    let res: Result<(), jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;
        let json_jstring = env.new_string(&bands_json)?;

        env.call_static_method(
            cls,
            "setEQBands",
            "(ZLjava/lang/String;)V",
            &[JValue::Bool(enabled as u8), JValue::Object(&json_jstring)],
        )?;

        Ok(())
    })();

    res.map_err(|e| e.to_string())
}

/// Configure crossfade. When enabled, ExoPlayer fires nativeOnNext() [secs]
/// seconds before the track ends; handlePlay() then cross-fades the volumes.
#[cfg(target_os = "android")]
pub fn set_crossfade_settings(enabled: bool, secs: f32) -> Result<(), String> {
    let jvm = get_jvm();
    let res: Result<(), jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;

        env.call_static_method(
            cls,
            "setCrossfadeSettings",
            "(ZF)V",
            &[JValue::Bool(enabled as u8), JValue::Float(secs)],
        )?;

        Ok(())
    })();

    res.map_err(|e| e.to_string())
}

/// Enable or disable gapless playback (reserved for future pre-buffering logic).
#[cfg(target_os = "android")]
pub fn set_gapless_enabled(enabled: bool) -> Result<(), String> {
    let jvm = get_jvm();
    let res: Result<(), jni::errors::Error> = (|| {
        let guard = jvm.attach_current_thread()?;
        let mut env = guard;

        let cls = get_companion_class(&mut env)?;

        env.call_static_method(
            cls,
            "setGaplessEnabled",
            "(Z)V",
            &[JValue::Bool(enabled as u8)],
        )?;

        Ok(())
    })();

    res.map_err(|e| e.to_string())
}

#[no_mangle]
#[cfg(target_os = "android")]
pub extern "system" fn Java_com_orb_app_MainActivity_nativeInit(
    mut env: JNIEnv,
    _class: JClass,
    class_loader: JObject,
    device_id: jni::objects::JString,
) {
    if let Ok(global) = env.new_global_ref(class_loader) {
        let _ = MEDIA_CLASSLOADER.set(global);
    }
    if let Ok(id) = env.get_string(&device_id) {
        let id_str: String = id.into();
        if !id_str.is_empty() {
            let _ = DEVICE_ID.set(id_str);
        }
    }
}

#[cfg(target_os = "android")]
pub fn get_device_id() -> Result<String, String> {
    Ok(DEVICE_ID.get().cloned().unwrap_or_default())
}
