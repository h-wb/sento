import org.jetbrains.kotlin.gradle.dsl.JvmTarget
import org.jetbrains.kotlin.gradle.tasks.KotlinJvmCompile

plugins {
    alias(libs.plugins.kotlin.jvm) apply false
    alias(libs.plugins.kotlin.serialization) apply false
    // Applied by the submodule's AndroidCompat build files.
    alias(libs.plugins.ktlint) apply false
}

subprojects {
    group = "sento"
    version = "0.1.0"

    plugins.withType<JavaPlugin> {
        extensions.configure<JavaPluginExtension> {
            val javaVersion = JavaVersion.toVersion(libs.versions.jvmTarget.get())
            sourceCompatibility = javaVersion
            targetCompatibility = javaVersion
        }
    }

    tasks.withType<KotlinJvmCompile> {
        compilerOptions {
            jvmTarget = JvmTarget.fromTarget(libs.versions.jvmTarget.get())
            // Upstream Suwayomi compiles with context parameters enabled;
            // the vendored code expects it.
            freeCompilerArgs.add("-Xcontext-parameters")
        }
    }
}
