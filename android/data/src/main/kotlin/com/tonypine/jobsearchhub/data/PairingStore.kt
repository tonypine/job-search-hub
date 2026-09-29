package com.tonypine.jobsearchhub.data

import android.content.Context
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import com.tonypine.jobsearchhub.core.Pairing

/**
 * Keeps the phone's pairing: the hub's address and the device token, the
 * token encrypted by a key in the Android Keystore.
 */
class PairingStore(context: Context) {
    private val preferences = EncryptedSharedPreferences.create(
        context,
        "pairing",
        MasterKey.Builder(context).setKeyScheme(MasterKey.KeyScheme.AES256_GCM).build(),
        EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
        EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
    )

    fun load(): Pairing? {
        val hubUrl = preferences.getString(HUB_URL, null) ?: return null
        val token = preferences.getString(TOKEN, null) ?: return null
        return Pairing(hubUrl, token)
    }

    fun save(pairing: Pairing) {
        preferences.edit().putString(HUB_URL, pairing.hubUrl).putString(TOKEN, pairing.token).apply()
    }

    fun forget() {
        preferences.edit().clear().apply()
    }

    private companion object {
        const val HUB_URL = "hub_url"
        const val TOKEN = "token"
    }
}
