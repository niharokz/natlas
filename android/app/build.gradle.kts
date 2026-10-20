// The app module: a WebView shell plus the Natlas engine (jniLibs/<abi>/libnatlas.so,
// built from ../cmd/natlas by scripts/build-in-container.sh) and native widgets.
plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// App version = engine version (the VERSION file at the repo root), so phone and server match.
val natlasVersion: String = (findProperty("natlasVersion") as String?)
    ?: rootProject.file("../VERSION").takeIf { it.exists() }?.readText()?.trim()
    ?: "0.0.0"

// 1.0.0 → 110000. Version numbers restarted at 1.0.0 after 3.1.1 (code 30101): the
// 100000 "epoch" keeps every new code higher, so updates install over old builds.
fun versionCodeOf(v: String): Int {
    val p = v.split(".").map { it.takeWhile(Char::isDigit).toIntOrNull() ?: 0 } + listOf(0, 0, 0)
    return 100000 + p[0] * 10000 + p[1] * 100 + p[2]
}

// Release signing comes from the environment (android/.env), never from the repo.
val keystorePath: String? = System.getenv("NATLAS_KEYSTORE")?.takeIf { it.isNotBlank() }

android {
    namespace = "app.natlas"
    compileSdk = 34
    buildToolsVersion = "34.0.0"

    defaultConfig {
        applicationId = "app.natlas"
        minSdk = 31 // Android 12: scrollable widgets, widget checkboxes, BiometricPrompt with screen lock
        targetSdk = 34
        versionCode = versionCodeOf(natlasVersion)
        versionName = natlasVersion
    }

    signingConfigs {
        if (keystorePath != null) {
            create("release") {
                storeFile = file(keystorePath)
                storePassword = System.getenv("NATLAS_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("NATLAS_KEY_ALIAS") ?: "natlas"
                keyPassword = System.getenv("NATLAS_KEY_PASSWORD") ?: System.getenv("NATLAS_KEYSTORE_PASSWORD")
            }
        }
    }

    buildTypes {
        getByName("release") {
            isMinifyEnabled = false // tiny app; keeps the JavaScript bridge intact
            if (keystorePath != null) signingConfig = signingConfigs.getByName("release")
        }
        getByName("debug") {
            applicationIdSuffix = ".debug" // installs next to the release app
            versionNameSuffix = "-debug"
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
        allWarningsAsErrors = true
    }

    packaging {
        jniLibs {
            useLegacyPackaging = true // extract libnatlas.so to disk so it can be executed
        }
    }

    lint {
        checkReleaseBuilds = false
        abortOnError = false
    }
}

dependencies {
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.json:json:20240303") // android's org.json is a stub in unit tests
}
