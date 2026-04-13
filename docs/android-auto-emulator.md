# Android Auto Emulator Setup (Linux)

This guide covers setting up the Android Auto desktop head unit emulator on Linux and developing against it using `adb`.

## Prerequisites

- Android SDK installed (includes `adb`)
- Android emulator or physical Android device connected via USB
- Built APK file ready to deploy

## Installation & Setup

### 1. Locate the Android Auto Desktop Head Unit

On Linux, the Android Auto emulator is typically installed with the Android SDK extras. It's located in:

```bash
$ANDROID_HOME/extras/google/auto/desktop-head-unit
```

If not present, install it via Android Studio's SDK Manager:

- Open Android Studio → Tools → SDK Manager
- Go to the **SDK Tools** tab
- Check **Android Auto API Simulators** (or search for "auto")
- Click **Apply** and install

### 2. Start the Emulator

Run the desktop head unit:

```bash
$ANDROID_HOME/extras/google/auto/desktop-head-unit
```

Or if you have it in your PATH:

```bash
desktop-head-unit
```

The emulator window should appear, showing the Android Auto interface.

## Working with adb

### Port Forwarding

Android Auto communicates over TCP. Set up port forwarding to connect your running emulator to the Orb server:

```bash
adb forward tcp:5277 tcp:5277
```

This forwards the emulator's port 5277 to the host's port 5277 (adjust if your server uses a different port).

To verify the forwarding is active:

```bash
adb forward --list
```

You should see output like:

```text
emulator-5554 tcp:5277 tcp:5277
```

### Installing the APK

After building the APK, install it to the emulator:

```bash
adb install app-universal-release.apk
```

For reinstall (if the app is already installed):

```bash
adb install -r app-universal-release.apk
```

To uninstall before a fresh install:

```bash
adb uninstall com.orb.app && adb install app-universal-release.apk
```

## Full Workflow

### Build & Deploy

From the repository root:

```bash
# Build the Android APK
make tauri-android-build

# Forward the port
adb forward tcp:5277 tcp:5277

# Install the APK (adjust path if needed)
adb install web/src-tauri/gen/android/app/build/outputs/apk/universal/release/app-universal-release.apk
```

### Launch in Android Auto Emulator

1. Ensure the desktop head unit is running
2. On the emulator screen, open the Orb app (may take a few seconds)
3. Configure the server URL to point to your Orb instance (default: `http://localhost:5277`)
4. Sign in and test

### Debugging

View emulator logs:

```bash
adb logcat | grep -i orb
```

To clear logs:

```bash
adb logcat -c
```

To see all connected devices/emulators:

```bash
adb devices
```

## Tips

- If the emulator doesn't detect the app, restart it and reinstall
- Use `adb shell` to open a shell on the emulator for manual testing
- Port forwarding persists until you run `adb forward --remove all` or disconnect the device
- The desktop head unit is CPU-intensive; close other apps if performance lags

## Related Commands

```bash
# List all forwarded ports
adb forward --list

# Remove a specific forward
adb forward --remove tcp:5277

# Remove all forwards
adb forward --remove-all

# Reboot the emulator
adb reboot

# Pull a file from emulator
adb pull /path/on/emulator /local/path

# Push a file to emulator
adb push /local/file /path/on/emulator
```
