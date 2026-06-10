plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    // Change this before publishing — must be globally unique on the Play Store.
    namespace = "com.irgo.{{PROJECT_IDENT}}"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.irgo.{{PROJECT_IDENT}}"
        minSdk = 24
        targetSdk = 35
        versionCode = 1
        versionName = "1.0"
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
        }
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }
}

dependencies {
    implementation(files("libs/irgo.aar"))
    implementation("androidx.core:core-ktx:1.13.1")
    implementation("androidx.appcompat:appcompat:1.7.0")
    implementation("com.google.android.material:material:1.12.0")
}
