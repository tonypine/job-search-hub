package com.tonypine.jobsearchhub.ui

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Checklist
import androidx.compose.material.icons.outlined.Home
import androidx.compose.material.icons.outlined.ViewKanban
import androidx.compose.material.icons.outlined.Work
import androidx.compose.material.icons.rounded.Checklist
import androidx.compose.material.icons.rounded.Home
import androidx.compose.material.icons.rounded.Settings
import androidx.compose.material.icons.rounded.ViewKanban
import androidx.compose.material.icons.rounded.Work
import androidx.compose.material3.Badge
import androidx.compose.material3.BadgedBox
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.core.content.ContextCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import com.tonypine.jobsearchhub.HubViewModel
import com.tonypine.jobsearchhub.PipelineFocus
import com.tonypine.jobsearchhub.core.PipelineCard
import com.tonypine.jobsearchhub.core.PipelinePhase
import com.tonypine.jobsearchhub.core.QueueTaskRequest
import com.tonypine.jobsearchhub.push.UpdateNotifications
import com.tonypine.jobsearchhub.ui.design.HubAction
import kotlinx.coroutines.launch

private const val TODAY = "today"
private const val DECIDE = "decide"
private const val PIPELINE = "pipeline"
private const val JOBS = "jobs"
private const val UPDATES = "updates"
private const val SETTINGS = "settings"
private const val JOB = "job/{id}?fromQueue={fromQueue}"
private const val COMPANY = "company/{id}"

/** A page of the navigation bar. */
private data class TopLevel(val route: String, val label: String, val icon: ImageVector, val selectedIcon: ImageVector)

private val topLevels = listOf(
    TopLevel(TODAY, "Today", Icons.Outlined.Home, Icons.Rounded.Home),
    TopLevel(DECIDE, "Decide", Icons.Outlined.Checklist, Icons.Rounded.Checklist),
    TopLevel(PIPELINE, "Pipeline", Icons.Outlined.ViewKanban, Icons.Rounded.ViewKanban),
    TopLevel(JOBS, "Jobs", Icons.Outlined.Work, Icons.Rounded.Work),
)

/** Pairing first; then Today, Decide, Pipeline and Jobs, a job's or company's details, the updates' history and Settings. */
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
    val snackbar = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()
    val say: (String) -> Unit = { message -> scope.launch { snackbar.showSnackbar(message) } }
    val goTo: (String) -> Unit = { destination ->
        navigation.navigate(destination) {
            popUpTo(navigation.graph.findStartDestination().id) { saveState = true }
            launchSingleTop = true
            restoreState = true
        }
    }
    val menu = listOf(HubAction("Settings", Icons.Rounded.Settings) { navigation.navigate(SETTINGS) { launchSingleTop = true } })
    val openCard: (PipelineCard) -> Unit = { card ->
        card.application.jobId?.let { navigation.navigate("job/$it") } ?: card.application.companyId?.let { navigation.navigate("company/$it") }
    }
    val followedUp: (PipelineCard, String) -> Unit = { card, note ->
        scope.launch {
            viewModel.recordFollowUp(card, note).fold({ say("Followed up on ${card.title}") }, { say(it.message ?: "Couldn't record the follow-up.") })
        }
    }
    val move: (PipelineCard, PipelinePhase, String) -> Unit = { card, phase, reason ->
        scope.launch {
            viewModel.moveCard(card, phase, reason).fold({ say("Moved ${card.title} to ${phase.name}") }, { say(it.message ?: "Couldn't move the card.") })
        }
    }
    val dueFollowUps = state.dueFollowUps().size
    Scaffold(
        snackbarHost = { SnackbarHost(snackbar) },
        bottomBar = {
            if (topLevels.any { it.route == route }) {
                NavigationBar {
                    topLevels.forEach { page ->
                        val count = when (page.route) {
                            DECIDE -> state.decisionQueue.size
                            PIPELINE -> dueFollowUps
                            else -> 0
                        }
                        NavigationBarItem(
                            selected = route == page.route, onClick = { goTo(page.route) }, label = { Text(page.label) },
                            icon = {
                                BadgedBox(badge = { if (count > 0) Badge { Text("$count") } }) {
                                    Icon(if (route == page.route) page.selectedIcon else page.icon, contentDescription = null)
                                }
                            },
                        )
                    }
                }
            }
        },
    ) { padding ->
        NavHost(navigation, startDestination = TODAY, modifier = Modifier.padding(padding)) {
            composable(TODAY) {
                TodayScreen(
                    state, onRefresh = viewModel::refresh,
                    onOpenDecisionJob = { navigation.navigate("job/$it?fromQueue=true") }, onSeeDecide = { goTo(DECIDE) },
                    onOpenCard = openCard, onSeePipeline = { goTo(PIPELINE) }, onFollowedUp = followedUp,
                    onOpenUpdate = { update ->
                        viewModel.markSeen(update)
                        when {
                            update.jobId != null -> navigation.navigate("job/${update.jobId}")
                            update.companyId != null -> navigation.navigate("company/${update.companyId}")
                            else -> navigation.navigate(UPDATES)
                        }
                    },
                    onSeeUpdates = { navigation.navigate(UPDATES) },
                    onOpenCompany = { navigation.navigate("company/$it") },
                    onAskTheMac = { company ->
                        scope.launch {
                            viewModel.askTheMac(QueueTaskRequest(kind = "research_company", company = company))
                                .fold({ say("Sent to the Mac. The result comes as an update.") }, { say(it.message ?: "Couldn't reach the Mac.") })
                        }
                    },
                    menu = menu,
                )
            }
            composable(DECIDE) {
                DecideScreen(state, onRefresh = viewModel::refresh, onOpenJob = { navigation.navigate("job/$it?fromQueue=true") }, menu = menu)
            }
            composable(PIPELINE) {
                val focus by viewModel.pipelineFocus.collectAsStateWithLifecycle()
                PipelineScreen(
                    state, focus = focus, onFocusShown = viewModel::clearPipelineFocus, onRefresh = viewModel::refresh,
                    onOpenCard = openCard, onFollowedUp = followedUp, onMove = move, menu = menu,
                )
            }
            composable(JOBS) {
                JobsScreen(
                    state, onRefresh = viewModel::refresh, onIncludeUnclear = viewModel::setIncludesUnclear,
                    onOpenJob = { navigation.navigate("job/$it") }, menu = menu,
                )
            }
            composable(UPDATES) {
                UpdatesScreen(
                    state, onBack = { navigation.popBackStack() }, onRefresh = viewModel::refresh, onSeen = viewModel::markSeen,
                    onOpenJob = { navigation.navigate("job/$it") }, onOpenCompany = { navigation.navigate("company/$it") },
                )
            }
            composable(SETTINGS) {
                SettingsScreen(state, onBack = { navigation.popBackStack() }, onUnpair = viewModel::unpair)
            }
            composable(
                JOB,
                arguments = listOf(
                    navArgument("id") { type = NavType.StringType },
                    navArgument("fromQueue") { type = NavType.BoolType; defaultValue = false },
                ),
            ) { backStack ->
                val fromQueue = backStack.arguments?.getBoolean("fromQueue") == true
                JobScreen(
                    backStack.arguments?.getString("id").orEmpty(), viewModel, onBack = { navigation.popBackStack() },
                    onOpenCompany = { navigation.navigate("company/$it") },
                    onDecided = { next ->
                        // From the queue, a decision moves on to the next job in it; elsewhere it goes back.
                        if (fromQueue && next != null) {
                            navigation.navigate("job/$next?fromQueue=true") { popUpTo(JOB) { inclusive = true } }
                        } else {
                            navigation.popBackStack()
                        }
                    },
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
            when {
                // A follow-up reminder opens its card on Pipeline, where it can be marked followed up.
                target.isFollowUp -> {
                    viewModel.focusPipeline(PipelineFocus(target.jobId, target.companyId))
                    goTo(PIPELINE)
                }
                target.jobId != null -> navigation.navigate("job/${target.jobId}")
                target.companyId != null -> navigation.navigate("company/${target.companyId}")
            }
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
