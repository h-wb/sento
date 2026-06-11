plugins {
    id(libs.plugins.kotlin.jvm.get().pluginId)
    id(libs.plugins.kotlin.serialization.get().pluginId)
    application
}

dependencies {
    implementation(libs.bundles.shared)
    implementation(project(":AndroidCompat"))
    implementation(project(":AndroidCompat:Config"))

    // Extension runtime surface (okhttp is version-locked by extensions)
    implementation(libs.bundles.okhttp)
    implementation(libs.injekt)

    // HTTP API
    implementation(libs.javalin.core)
}

application {
    mainClass.set("sento.host.MainKt")
}
