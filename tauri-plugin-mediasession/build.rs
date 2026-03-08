const COMMANDS: &[&str] = &[
    "initialize_player",
    "load_track",
    "play",
    "pause",
    "next_track",
    "previous_track",
    "seek",
    "stop",
    "get_log",
    "write_log",
];

fn main() {
    tauri_plugin::Builder::new(COMMANDS)
        .android_path("android")
        .build();
}
