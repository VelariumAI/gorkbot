package vision

// companion.go — source code and build system for the Gorkbot Vision companion APK.
//
// The companion APK is a minimal Android application that:
//   1. Uses the MediaProjection API (Android's only non-root screen capture path)
//   2. Runs a foreground service with a plain TCP/HTTP server on 127.0.0.1:7777
//   3. Auto-starts on boot
//
// Build chain (all available in Termux):
//   pkg install openjdk-21 gradle
//   Then the vision_install tool runs `gradle assembleDebug` in the generated project.
//
// NOTHING here depends on ADB.

// companionPort is the localhost port the companion service listens on.
const companionPort = 7777

// companionPkg is the Android package name of the companion app.
const companionPkg = "ai.velarium.gorkbot"

// ─── Embedded project source files ───────────────────────────────────────────

const companionSettingsGradle = `rootProject.name = 'gorkbot-companion'
include ':app'
`

const companionRootGradle = `buildscript {
    repositories {
        google()
        mavenCentral()
    }
    dependencies {
        classpath 'com.android.tools.build:gradle:8.2.0'
    }
}

allprojects {
    repositories {
        google()
        mavenCentral()
    }
}
`

const companionAppGradle = `plugins {
    id 'com.android.application'
}

android {
    namespace 'ai.velarium.gorkbot'
    compileSdk 34

    defaultConfig {
        applicationId 'ai.velarium.gorkbot'
        minSdk 29
        targetSdk 34
        versionCode 2
        versionName '1.0.2'
    }

    buildTypes {
        debug {
            minifyEnabled false
            debuggable true
        }
    }

    compileOptions {
        sourceCompatibility JavaVersion.VERSION_1_8
        targetCompatibility JavaVersion.VERSION_1_8
    }
}
`

const companionManifest = `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="ai.velarium.gorkbot">

    <uses-permission android:name="android.permission.FOREGROUND_SERVICE" />
    <uses-permission android:name="android.permission.FOREGROUND_SERVICE_MEDIA_PROJECTION" />
    <uses-permission android:name="android.permission.POST_NOTIFICATIONS" />
    <uses-permission android:name="android.permission.RECEIVE_BOOT_COMPLETED" />

    <application
        android:label="Gorkbot Vision"
        android:allowBackup="false">

        <!-- Launched to request MediaProjection permission from the user (one time) -->
        <activity
            android:name=".PermissionActivity"
            android:exported="true"
            android:theme="@android:style/Theme.Translucent.NoTitleBar"
            android:excludeFromRecents="true" />

        <!-- Foreground service that serves screenshots over localhost:7777 -->
        <service
            android:name=".ScreenService"
            android:foregroundServiceType="mediaProjection"
            android:exported="false" />

        <!-- Auto-start on device boot -->
        <receiver
            android:name=".BootReceiver"
            android:exported="true">
            <intent-filter>
                <action android:name="android.intent.action.BOOT_COMPLETED" />
            </intent-filter>
        </receiver>

    </application>
</manifest>
`

const companionPermissionActivity = `package ai.velarium.gorkbot;

import android.app.Activity;
import android.content.Intent;
import android.media.projection.MediaProjectionManager;
import android.os.Bundle;
import android.util.Log;

/**
 * Transparent activity whose only job is to show Android's MediaProjection
 * consent dialog. The user taps "Start now" once; after that the ScreenService
 * runs forever until the phone reboots (and BootReceiver relaunches it).
 */
public class PermissionActivity extends Activity {
    private static final String TAG = "GorkbotVision";
    private static final int REQ_CODE = 100;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        MediaProjectionManager mpm =
            (MediaProjectionManager) getSystemService(MEDIA_PROJECTION_SERVICE);
        startActivityForResult(mpm.createScreenCaptureIntent(), REQ_CODE);
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        if (requestCode == REQ_CODE && resultCode == RESULT_OK) {
            Log.i(TAG, "MediaProjection permission granted — starting ScreenService");
            Intent svc = new Intent(this, ScreenService.class);
            svc.putExtra("result_code", resultCode);
            svc.putExtra("data", data);
            startForegroundService(svc);
        } else {
            Log.w(TAG, "MediaProjection permission denied or cancelled");
        }
        finish();
    }
}
`

const companionScreenService = `package ai.velarium.gorkbot;

import android.app.*;
import android.content.Context;
import android.content.Intent;
import android.graphics.Bitmap;
import android.graphics.PixelFormat;
import android.hardware.display.DisplayManager;
import android.hardware.display.VirtualDisplay;
import android.media.Image;
import android.media.ImageReader;
import android.media.projection.MediaProjection;
import android.media.projection.MediaProjectionManager;
import android.os.Build;
import android.os.IBinder;
import android.util.DisplayMetrics;
import android.util.Log;
import android.view.WindowManager;

import java.io.*;
import java.net.*;
import java.nio.ByteBuffer;

/**
 * Foreground service that captures the screen via MediaProjection and serves
 * PNG screenshots over a plain HTTP server on 127.0.0.1:7777.
 *
 * API:
 *   GET /screenshot  → image/png  (current screen contents)
 *   GET /status      → application/json  ({"status":"running"})
 *   GET /stop        → stops the service
 */
public class ScreenService extends Service {
    private static final String TAG      = "GorkbotVision";
    private static final int    PORT     = 7777;
    private static final int    NOTIF_ID = 1;
    private static final String CHAN_ID  = "gorkbot_vision";

    private MediaProjection projection;
    private ServerSocket    serverSocket;
    private volatile boolean running = false;

    // ── Lifecycle ─────────────────────────────────────────────────────────────

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        createNotificationChannel();
        startForeground(NOTIF_ID, buildNotification());

        int    resultCode = intent.getIntExtra("result_code", -1);
        Intent data       = intent.getParcelableExtra("data");

        MediaProjectionManager mpm =
            (MediaProjectionManager) getSystemService(MEDIA_PROJECTION_SERVICE);
        projection = mpm.getMediaProjection(resultCode, data);

        running = true;
        new Thread(this::runHttpServer, "gorkbot-http").start();
        Log.i(TAG, "ScreenService started on port " + PORT);
        return START_STICKY;
    }

    @Override
    public IBinder onBind(Intent intent) { return null; }

    @Override
    public void onDestroy() {
        running = false;
        if (projection != null) { projection.stop(); projection = null; }
        if (serverSocket != null) {
            try { serverSocket.close(); } catch (IOException ignored) {}
        }
        Log.i(TAG, "ScreenService stopped");
    }

    // ── HTTP server ───────────────────────────────────────────────────────────

    private void runHttpServer() {
        try {
            serverSocket = new ServerSocket(PORT, 8,
                InetAddress.getByName("127.0.0.1"));
            Log.i(TAG, "Listening on 127.0.0.1:" + PORT);
            while (running) {
                Socket client = serverSocket.accept();
                new Thread(() -> handleClient(client), "gorkbot-req").start();
            }
        } catch (IOException e) {
            if (running) Log.e(TAG, "Server error: " + e.getMessage());
        }
    }

    private void handleClient(Socket client) {
        try {
            // Read the HTTP request line
            BufferedReader reader = new BufferedReader(
                new InputStreamReader(client.getInputStream()));
            String requestLine = reader.readLine();
            if (requestLine == null) { client.close(); return; }

            byte[] body;
            String contentType;

            if (requestLine.startsWith("GET /screenshot")) {
                body        = captureScreenPng();
                contentType = "image/png";
            } else if (requestLine.startsWith("GET /status")) {
                body        = ("{\"status\":\"running\",\"port\":" + PORT + "}").getBytes("UTF-8");
                contentType = "application/json";
            } else if (requestLine.startsWith("GET /stop")) {
                body        = "stopping".getBytes("UTF-8");
                contentType = "text/plain";
                sendHttpResponse(client, 200, contentType, body);
                client.close();
                stopSelf();
                return;
            } else {
                body        = "404 not found".getBytes("UTF-8");
                contentType = "text/plain";
                sendHttpResponse(client, 404, contentType, body);
                client.close();
                return;
            }

            sendHttpResponse(client, 200, contentType, body);
        } catch (Exception e) {
            Log.e(TAG, "Request error: " + e.getMessage());
        } finally {
            try { client.close(); } catch (IOException ignored) {}
        }
    }

    private void sendHttpResponse(Socket client, int status, String ct, byte[] body)
            throws IOException {
        OutputStream out = client.getOutputStream();
        String statusText = (status == 200) ? "OK" : "Not Found";
        String header = "HTTP/1.1 " + status + " " + statusText + "\r\n"
            + "Content-Type: " + ct + "\r\n"
            + "Content-Length: " + body.length + "\r\n"
            + "Connection: close\r\n\r\n";
        out.write(header.getBytes("UTF-8"));
        out.write(body);
        out.flush();
    }

    // ── Screen capture ────────────────────────────────────────────────────────

    private byte[] captureScreenPng() throws Exception {
        WindowManager wm = (WindowManager) getSystemService(WINDOW_SERVICE);
        DisplayMetrics dm = new DisplayMetrics();
        wm.getDefaultDisplay().getRealMetrics(dm);
        int w = dm.widthPixels, h = dm.heightPixels, dpi = dm.densityDpi;

        ImageReader reader = ImageReader.newInstance(w, h, PixelFormat.RGBA_8888, 2);
        VirtualDisplay vd = projection.createVirtualDisplay(
            "gorkbot-capture", w, h, dpi,
            DisplayManager.VIRTUAL_DISPLAY_FLAG_AUTO_MIRROR,
            reader.getSurface(), null, null);

        // Give the virtual display one frame to populate
        Thread.sleep(120);
        Image image = reader.acquireLatestImage();

        // Retry once if first frame not ready
        if (image == null) {
            Thread.sleep(200);
            image = reader.acquireLatestImage();
        }

        if (image == null) {
            vd.release();
            reader.close();
            throw new Exception("VirtualDisplay produced no image — MediaProjection may have expired");
        }

        try {
            Image.Plane plane     = image.getPlanes()[0];
            ByteBuffer  buffer    = plane.getBuffer();
            int         pxStride  = plane.getPixelStride();
            int         rowStride = plane.getRowStride();

            // Account for row padding
            int paddedW = rowStride / pxStride;
            Bitmap bmp  = Bitmap.createBitmap(paddedW, h, Bitmap.Config.ARGB_8888);
            bmp.copyPixelsFromBuffer(buffer);

            // Crop to actual screen dimensions
            if (paddedW != w) {
                bmp = Bitmap.createBitmap(bmp, 0, 0, w, h);
            }

            ByteArrayOutputStream baos = new ByteArrayOutputStream();
            bmp.compress(Bitmap.CompressFormat.PNG, 100, baos);
            bmp.recycle();
            return baos.toByteArray();
        } finally {
            image.close();
            vd.release();
            reader.close();
        }
    }

    // ── Notification (required for foreground service) ────────────────────────

    private void createNotificationChannel() {
        NotificationChannel ch = new NotificationChannel(
            CHAN_ID, "Gorkbot Vision", NotificationManager.IMPORTANCE_LOW);
        ch.setDescription("Screen capture service for Gorkbot AI");
        ((NotificationManager) getSystemService(NOTIFICATION_SERVICE))
            .createNotificationChannel(ch);
    }

    private Notification buildNotification() {
        return new Notification.Builder(this, CHAN_ID)
            .setContentTitle("Gorkbot Vision")
            .setContentText("Screen capture ready on port " + PORT)
            .setSmallIcon(android.R.drawable.ic_menu_camera)
            .setOngoing(true)
            .build();
    }
}
`

const companionBootReceiver = `package ai.velarium.gorkbot;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.util.Log;

/**
 * Relaunches PermissionActivity on device boot so the user can re-grant
 * MediaProjection if needed (Android revokes it on reboot).
 */
public class BootReceiver extends BroadcastReceiver {
    @Override
    public void onReceive(Context context, Intent intent) {
        if (Intent.ACTION_BOOT_COMPLETED.equals(intent.getAction())) {
            Log.i("GorkbotVision", "Boot completed — launching PermissionActivity");
            Intent i = new Intent(context, PermissionActivity.class);
            i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
            context.startActivity(i);
        }
    }
}
`

// companionGitignore keeps generated files out of any git tree in the project dir.
const companionGitignore = `
.gradle/
build/
local.properties
*.apk
`

// ─── Exported accessors for vision_install tool ───────────────────────────────

func CompanionSettingsGradle() string    { return companionSettingsGradle }
func CompanionRootGradle() string        { return companionRootGradle }
func CompanionAppGradle() string         { return companionAppGradle }
func CompanionManifest() string          { return companionManifest }
func CompanionPermissionActivity() string { return companionPermissionActivity }
func CompanionScreenService() string     { return companionScreenService }
func CompanionBootReceiver() string      { return companionBootReceiver }
func CompanionGitignore() string         { return companionGitignore }
