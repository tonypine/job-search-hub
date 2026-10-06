package com.tonypine.jobsearchhub.data

import com.tonypine.jobsearchhub.core.GitHubRelease
import com.tonypine.jobsearchhub.core.hubJson
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import java.io.File
import java.io.IOException
import java.security.MessageDigest
import java.util.concurrent.TimeUnit

/**
 * Reads the repository's releases from GitHub's public API, without a token, and downloads their assets. The phone
 * asks GitHub itself, so it learns of a new version even when the Mac is off.
 */
class ReleaseClient(
    private val repository: String = REPOSITORY,
    private val apiUrl: String = "https://api.github.com",
    private val http: OkHttpClient = OkHttpClient.Builder().connectTimeout(20, TimeUnit.SECONDS).readTimeout(60, TimeUnit.SECONDS).build(),
) {
    /** The newest releases, Mac and Android alike, drafts and pre-releases included, newest first. */
    suspend fun getReleases(): List<GitHubRelease> = hubJson.decodeFromString(text("$apiUrl/repos/$repository/releases?per_page=100"))

    /** A small asset's text, such as an APK's `.sha256`. */
    suspend fun text(url: String): String = withContext(Dispatchers.IO) {
        execute(url).use { response ->
            if (!response.isSuccessful) throw IOException("GitHub answered ${response.code} for $url.")
            response.body.string()
        }
    }

    /**
     * Downloads [url] into [file], reporting the fraction done when the size is known, and returns the file's
     * SHA-256 in hex. A failed or cancelled download leaves no file behind.
     */
    suspend fun download(url: String, file: File, onProgress: (Float) -> Unit = {}): String = withContext(Dispatchers.IO) {
        val digest = MessageDigest.getInstance("SHA-256")
        try {
            execute(url).use { response ->
                if (!response.isSuccessful) throw IOException("GitHub answered ${response.code} for $url.")
                val size = response.body.contentLength()
                file.parentFile?.mkdirs()
                response.body.byteStream().use { input ->
                    file.outputStream().use { output ->
                        val buffer = ByteArray(64 * 1024)
                        var done = 0L
                        while (true) {
                            ensureActive()
                            val read = input.read(buffer)
                            if (read < 0) break
                            output.write(buffer, 0, read)
                            digest.update(buffer, 0, read)
                            done += read
                            if (size > 0) onProgress(done.toFloat() / size)
                        }
                    }
                }
            }
            digest.digest().joinToString("") { "%02x".format(it) }
        } catch (error: Throwable) {
            file.delete()
            throw error
        }
    }

    private fun execute(url: String): Response {
        val request = Request.Builder()
            .url(url)
            .header("Accept", "application/vnd.github+json")
            .header("X-GitHub-Api-Version", "2022-11-28")
            .build()
        return try {
            http.newCall(request).execute()
        } catch (error: IOException) {
            throw IOException("Can't reach GitHub: ${error.message}", error)
        }
    }

    companion object {
        /** Where the app's releases are published. */
        const val REPOSITORY = "tonypine/job-search-hub"
    }
}
