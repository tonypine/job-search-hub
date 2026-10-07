plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
}

// The Firebase project's config stays out of the repo. Without it the app
// builds and runs, with pushes off.
if (file("google-services.json").exists()) {
    apply(plugin = libs.plugins.google.services.get().pluginId)
}

// Releases are <versionMajorMinor>.<versionCode>, and the release workflow passes
// -PversionCode (the commit count on main) and -PversionName. Raise this by hand for
// a new major or minor version. scripts/release/version.sh reads it from here, so the
// Mac app and the server carry the same version.
val versionMajorMinor = "0.1"

// Any other build is a dev version of its commit, as version.sh names it.
val devVersionName = runCatching {
    providers.exec { commandLine("git", "rev-parse", "--short", "HEAD") }.standardOutput.asText.get().trim()
}.getOrNull()?.takeIf { it.isNotEmpty() }.let { commit -> "$versionMajorMinor.0-dev" + (commit?.let { ".$it" } ?: "") }

// The release key comes from the environment, all four values or none. With none the
// release APK is unsigned, which the release workflow refuses to publish.
val releaseSigning = listOf(
    "RELEASE_KEYSTORE_PATH",
    "RELEASE_KEYSTORE_PASSWORD",
    "RELEASE_KEY_ALIAS",
    "RELEASE_KEY_PASSWORD",
).associateWith { providers.environmentVariable(it).orNull.orEmpty() }
val missingReleaseSigning = releaseSigning.filterValues { it.isEmpty() }.keys
if (missingReleaseSigning.isNotEmpty() && missingReleaseSigning.size < releaseSigning.size) {
    throw GradleException(
        "Release signing needs all of ${releaseSigning.keys.joinToString()}; " +
            "missing ${missingReleaseSigning.joinToString()}",
    )
}

android {
    namespace = "com.tonypine.jobsearchhub"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.tonypine.jobsearchhub"
        minSdk = 30
        targetSdk = 36
        versionCode = providers.gradleProperty("versionCode").map(String::toInt).getOrElse(1)
        versionName = providers.gradleProperty("versionName").getOrElse(devVersionName)
    }

    signingConfigs {
        if (missingReleaseSigning.isEmpty()) {
            create("release") {
                storeFile = file(releaseSigning.getValue("RELEASE_KEYSTORE_PATH"))
                storePassword = releaseSigning.getValue("RELEASE_KEYSTORE_PASSWORD")
                keyAlias = releaseSigning.getValue("RELEASE_KEY_ALIAS")
                keyPassword = releaseSigning.getValue("RELEASE_KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        release {
            signingConfig = signingConfigs.findByName("release")
        }
    }

    buildFeatures {
        compose = true
        // BuildConfig.VERSION_NAME, which the app names itself to the hub with.
        buildConfig = true
    }

    // Findings from before CI ran lint. New ones still fail `lintDebug`.
    lint {
        baseline = file("lint-baseline.xml")
    }

    testOptions {
        unitTests {
            // Android's classes, like Intent, are stubs on the JVM; this makes them return defaults instead of throwing.
            isReturnDefaultValues = true
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_21
        targetCompatibility = JavaVersion.VERSION_21
    }
}

// Each variant's unit tests read the manifest Gradle merged for its APK (ManifestTest).
androidComponents {
    onVariants { variant ->
        val manifest = variant.artifacts.get(com.android.build.api.artifact.SingleArtifact.MERGED_MANIFEST)
        val testTask = "test${variant.name.replaceFirstChar(Char::uppercase)}UnitTest"
        tasks.withType<Test>().matching { it.name == testTask }.configureEach {
            inputs.file(manifest).withPathSensitivity(PathSensitivity.NONE)
            jvmArgumentProviders.add(CommandLineArgumentProvider { listOf("-DmergedManifest=${manifest.get().asFile.absolutePath}") })
        }
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_21)
    }
}

dependencies {
    implementation(project(":data"))
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.material3)
    implementation(libs.compose.material3.adaptive.navigation.suite)
    implementation(libs.compose.material3.adaptive.layout)
    implementation(libs.compose.material.icons)
    implementation(libs.navigation.compose)
    implementation(libs.work.runtime.ktx)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.play.services.code.scanner)
    implementation(platform(libs.firebase.bom))
    implementation(libs.firebase.messaging)
    implementation(libs.kotlinx.coroutines.play.services)
    debugImplementation(libs.compose.ui.tooling)

    testImplementation(libs.kotlin.test)
    testImplementation(libs.junit)
}
