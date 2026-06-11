package sento.host

// Source preferences (ConfigurableSource): the extension declares a
// PreferenceScreen; we serialize it to a typed JSON form and write
// values back into the source's fake-Android SharedPreferences. The
// approach mirrors Suwayomi's Source.kt, but addressed by preference key
// (stable across extension versions) instead of screen position —
// Sento replays stored values onto fresh hosts.

import android.app.Application
import androidx.preference.ListPreference
import androidx.preference.MultiSelectListPreference
import androidx.preference.Preference
import androidx.preference.PreferenceScreen
import eu.kanade.tachiyomi.source.CatalogueSource
import eu.kanade.tachiyomi.source.ConfigurableSource
import eu.kanade.tachiyomi.source.sourcePreferences
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonPrimitive
import suwayomi.tachidesk.manga.impl.util.source.GetCatalogueSource
import uy.kohesive.injekt.Injekt
import uy.kohesive.injekt.api.get

@Serializable
data class PreferenceDto(
    val key: String,
    val type: String,
    val title: String? = null,
    val summary: String? = null,
    val valueType: String,
    val default: JsonElement = JsonNull,
    val current: JsonElement = JsonNull,
    val entries: List<String>? = null,
    val entryValues: List<String>? = null,
)

@Serializable
data class SetPreferenceBody(
    val key: String,
    val value: JsonElement,
)

object SourcePrefs {
    private fun screenOf(source: ConfigurableSource): PreferenceScreen {
        val screen = PreferenceScreen(Injekt.get<Application>())
        screen.sharedPreferences = source.sourcePreferences()
        source.setupPreferenceScreen(screen)
        return screen
    }

    fun list(source: CatalogueSource): List<PreferenceDto> {
        if (source !is ConfigurableSource) return emptyList()
        return screenOf(source).preferences.mapNotNull { pref ->
            val key = pref.key ?: return@mapNotNull null
            PreferenceDto(
                key = key,
                type = pref.javaClass.simpleName,
                title = pref.title?.toString(),
                summary = pref.summary?.toString(),
                valueType = pref.defaultValueType,
                default = pref.defaultValue.toJson(),
                current = pref.currentValue.toJson(),
                entries = (pref as? ListPreference)?.entries?.map { it.toString() }
                    ?: (pref as? MultiSelectListPreference)?.entries?.map { it.toString() },
                entryValues = (pref as? ListPreference)?.entryValues?.map { it.toString() }
                    ?: (pref as? MultiSelectListPreference)?.entryValues?.map { it.toString() },
            )
        }
    }

    // set writes one preference and bounces the source instance so it
    // re-reads its configuration.
    fun set(
        source: CatalogueSource,
        body: SetPreferenceBody,
    ) {
        require(source is ConfigurableSource) { "source ${source.id} has no preferences" }
        val pref =
            screenOf(source).preferences.firstOrNull { it.key == body.key }
                ?: throw IllegalArgumentException("source ${source.id} has no preference ${body.key}")
        val value = fromJson(pref, body.value)
        pref.saveNewValue(value)
        pref.callChangeListener(value)
        GetCatalogueSource.unregisterCatalogueSource(source.id)
    }

    private fun Any?.toJson(): JsonElement =
        when (this) {
            null -> JsonNull
            is Boolean -> JsonPrimitive(this)
            is Collection<*> -> JsonArray(map { JsonPrimitive(it.toString()) })
            else -> JsonPrimitive(toString())
        }

    private fun fromJson(
        pref: Preference,
        value: JsonElement,
    ): Any =
        when (pref.defaultValueType) {
            "Boolean" -> value.jsonPrimitive.boolean
            "Set<String>" -> value.jsonArray.map { it.jsonPrimitive.content }.toSet()
            else -> value.jsonPrimitive.content
        }
}
