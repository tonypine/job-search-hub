package com.tonypine.jobsearchhub.ui

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.List
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.core.content.ContextCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import com.tonypine.jobsearchhub.HubViewModel
import com.tonypine.jobsearchhub.core.QueueTaskRequest
import com.tonypine.jobsearchhub.push.UpdateNotifications

private const val UPDATES = "updates"
private const val JOBS = "jobs"
private const val JOB = "job/{id}"
private const val COMPANY = "company/{id}"

/** Pairing first; then Updates and Jobs, and a job's details. */
@Composable
fun HubNavigation(viewModel: HubViewModel) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    if (state.pairing == null) {
        PairScreen(error = state.error, onPair = viewModel::pair)
        return
    }
    AskToNotify()
    val navigation = rememberNavController()
    val entry by navigation.currentBackStackEntryAsState()
    val route = entry?.destination?.route
    Scaffold(
        bottomBar = {
            if (route != JOB && route != COMPANY) {
                NavigationBar {
                    NavigationBarItem(
                        selected = route == UPDATES, onClick = { navigation.navigate(UPDATES) { launchSingleTop = true } },
                        icon = { Icon(Icons.Filled.Notifications, contentDescription = null) }, label = { Text("Updates") },
                    )
                    NavigationBarItem(
                        selected = route == JOBS, onClick = { navigation.navigate(JOBS) { launchSingleTop = true } },
                        icon = { Icon(Icons.AutoMirrored.Filled.List, contentDescription = null) }, label = { Text("Jobs") },
                    )
                }
            }
        },
    ) { padding ->
        NavHost(navigation, startDestination = UPDATES, modifier = Modifier.padding(padding)) {
            composable(UPDATES) {
                UpdatesScreen(
                    state, onRefresh = viewModel::refresh, onUnpair = viewModel::unpair,
                    onOpenJob = { navigation.navigate("job/$it") }, onOpenCompany = { navigation.navigate("company/$it") },
                )
            }
            composable(JOBS) {
                JobsScreen(
                    state, onRefresh = viewModel::refresh, onIncludeUnclear = viewModel::setIncludesUnclear,
                    onOpenJob = { navigation.navigate("job/$it") },
                    onResearch = { company ->
                        viewModel.askTheMac(QueueTaskRequest(kind = "research_company", company = company))
                            .fold({ "Sent to the Mac. The result comes as an update." }, { it.message })
                    },
                )
            }
            composable(JOB, arguments = listOf(navArgument("id") { type = NavType.StringType })) { backStack ->
                JobScreen(
                    backStack.arguments?.getString("id").orEmpty(), viewModel, onBack = { navigation.popBackStack() },
                    onOpenCompany = { navigation.navigate("company/$it") },
                )
            }
            composable(COMPANY, arguments = listOf(navArgument("id") { type = NavType.StringType })) { backStack ->
                CompanyScreen(
                    backStack.arguments?.getString("id").orEmpty(), viewModel, onBack = { navigation.popBackStack() },
                    onOpenJob = { navigation.navigate("job/$it") },
                )
            }
        }
        val notificationTarget by viewModel.notificationTarget.collectAsStateWithLifecycle()
        LaunchedEffect(notificationTarget) {
            val target = notificationTarget ?: return@LaunchedEffect
            target.jobId?.let { navigation.navigate("job/$it") } ?: target.companyId?.let { navigation.navigate("company/$it") }
            viewModel.clearNotificationTarget()
        }
    }
}

/** Asks for the notification permission Android 13 and later want, when this build can get pushes. */
@Composable
private fun AskToNotify() {
    val context = LocalContext.current
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU || !UpdateNotifications.isAvailable(context)) {
        return
    }
    val request = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) {}
    LaunchedEffect(Unit) {
        if (ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            request.launch(Manifest.permission.POST_NOTIFICATIONS)
        }
    }
}
