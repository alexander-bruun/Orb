# Preserve the MediaService class and its static bridge methods
-keep class com.orb.app.MediaService {
    @kotlin.jvm.JvmStatic *;
    public *;
}

# Preserve the MainActivity's Companion where nativeInit lives
-keep class com.orb.app.MainActivity$Companion {
    public *;
}