const COMMANDS: &[&str] = &[
    "set_now_playing",
    "set_media_root",
    "set_playback_state",
    "on_car_connected",
];

fn main() {
    tauri_plugin::Builder::new(COMMANDS)
        .android_path("android")
        .build();
}
