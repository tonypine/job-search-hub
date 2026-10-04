package com.tonypine.jobsearchhub.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.QrCodeScanner
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.codescanner.GmsBarcodeScannerOptions
import com.google.mlkit.vision.codescanner.GmsBarcodeScanning
import com.tonypine.jobsearchhub.ui.design.Spacing

/** Pairs the phone with the hub through the Mac's QR code, or its link pasted. */
@Composable
fun PairScreen(error: String?, onPair: (String) -> Boolean) {
    val context = LocalContext.current
    var link by remember { mutableStateOf("") }
    var scanError by remember { mutableStateOf<String?>(null) }
    Column(
        modifier = Modifier.fillMaxSize().padding(Spacing.xl),
        verticalArrangement = Arrangement.spacedBy(Spacing.l, alignment = androidx.compose.ui.Alignment.CenterVertically),
    ) {
        Text("Pair with your hub", style = MaterialTheme.typography.headlineSmall)
        Text(
            "On the Mac, open Job Search Hub › Settings › Phones › Pair a phone, then scan its QR code.",
            style = MaterialTheme.typography.bodyMedium,
        )
        Button(onClick = {
            val options = GmsBarcodeScannerOptions.Builder().setBarcodeFormats(Barcode.FORMAT_QR_CODE).build()
            GmsBarcodeScanning.getClient(context, options).startScan()
                .addOnSuccessListener { barcode -> barcode.rawValue?.let(onPair) }
                .addOnFailureListener { scanError = "The scanner couldn't start: ${it.message}" }
        }, modifier = Modifier.fillMaxWidth()) {
            Icon(Icons.Filled.QrCodeScanner, contentDescription = null)
            Text("  Scan the QR code")
        }
        Text("Or paste the link shown under it:", style = MaterialTheme.typography.bodyMedium)
        OutlinedTextField(value = link, onValueChange = { link = it }, label = { Text("jobsearchhub://pair?…") }, modifier = Modifier.fillMaxWidth())
        OutlinedButton(onClick = { onPair(link) }, enabled = link.isNotBlank(), modifier = Modifier.fillMaxWidth()) { Text("Pair") }
        (scanError ?: error)?.let { Text(it, color = MaterialTheme.colorScheme.error) }
    }
}
