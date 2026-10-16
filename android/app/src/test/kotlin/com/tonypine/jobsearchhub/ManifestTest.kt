package com.tonypine.jobsearchhub

import java.io.File
import javax.xml.parsers.DocumentBuilderFactory
import kotlin.test.Test
import kotlin.test.assertTrue

class ManifestTest {
    @Test
    fun theMergedManifestLetsTheAppUpdateItselfWithoutAsking() {
        val permissions = mergedManifestPermissions()

        assertTrue("android.permission.REQUEST_INSTALL_PACKAGES" in permissions, "$permissions")
        assertTrue("android.permission.UPDATE_PACKAGES_WITHOUT_USER_ACTION" in permissions, "$permissions")
    }

    /** The permissions the manifest Gradle merged for this build declares, as the APK carries them. */
    private fun mergedManifestPermissions(): List<String> {
        // Set by app/build.gradle.kts to the variant's merged manifest.
        val manifest = File(System.getProperty("mergedManifest") ?: error("No mergedManifest: run the test through Gradle."))
        val document = DocumentBuilderFactory.newInstance().apply { isNamespaceAware = true }.newDocumentBuilder().parse(manifest)
        val elements = document.getElementsByTagName("uses-permission")
        return (0 until elements.length).map {
            elements.item(it).attributes.getNamedItemNS("http://schemas.android.com/apk/res/android", "name").nodeValue
        }
    }
}
