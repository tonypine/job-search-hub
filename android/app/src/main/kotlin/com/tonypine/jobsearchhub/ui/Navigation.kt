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
import androidx.compose.material3.Icon
import androidx.compose.material3.Scaffold
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteScaffold
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteType
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
private const val JOB = "job/{id}"
private const val COMPANY = "company/{id}"

/** A page of the navigation bar or rail. */
private data class TopLevel(val route: String, val label: String, val icon: ImageVector, val selectedIcon: ImageVector)

private val topLevels = listOf(
    TopLevel(TODAY, "Today", Icons.Outlined.Home, Icons.Rounded.Home),
    TopLevel(DECIDE, "Decide", Icons.Outlined.Checklist, Icons.Rounded.Checklist),
    TopLevel(PIPELINE, "Pipeline", Icons.Outlined.ViewKanban, Icons.Rounded.ViewKanban),
    TopLevel(JOBS, "Jobs", Icons.Outlined.Work, Icons.Rounded.Work),
)

/**
 * Pairing first; then Today, Decide, Pipeline and Jobs, a job's or company's details, the updates' history and Settings.
 * Under 600 dp a navigation bar switches pages and a job opens over its page; from 600 dp, on an unfolded Fold, a
 * tablet or a wide split screen, a rail does, and the job opens beside the page's list.
 */
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
    // Each page keeps its own open job or company, through rotating and folding.
    val todayDetails = rememberDetailStack()
    val decideDetails = rememberDetailStack()
    val pipelineDetails = rememberDetailStack()
    val jobsDetails = rememberDetailStack()
    val isWide = showsTwoPanes()
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
    // A page's open job or company, with a back arrow only when it shows alone.
    val detailPane: @Composable (DetailStack, Detail, Boolean) -> Unit = { details, detail, isAlone ->
        val onBack = if (isAlone) details::close else null
        when (detail.kind) {
            Detail.Kind.JOB -> JobScreen(
                detail.id, viewModel, onBack = onBack,
                onOpenCompany = { details.open(Detail.company(it)) },
                onDecided = { next ->
                    // From the queue, a decision moves on to the next job in it; elsewhere it closes the job.
                    if (detail.fromQueue && next != null) details.replace(Detail.job(next, fromQueue = true)) else details.close()
                },
            )
            Detail.Kind.COMPANY -> CompanyScreen(detail.id, viewModel, onBack = onBack, onOpenJob = { details.open(Detail.job(it)) })
        }
    }
    val page = topLevels.firstOrNull { it.route == route }
    val pageDetails = when (route) {
        TODAY -> todayDetails
        DECIDE -> decideDetails
        PIPELINE -> pipelineDetails
        JOBS -> jobsDetails
        else -> null
    }
    val dueFollowUps = state.dueFollowUps().size
    NavigationSuiteScaffold(
        layoutType = when {
            page == null -> NavigationSuiteType.None
            isWide -> NavigationSuiteType.NavigationRail
            // On a phone a job opens over its page, as before.
            pageDetails?.current != null -> NavigationSuiteType.None
            else -> NavigationSuiteType.NavigationBar
        },
        navigationSuiteItems = {
            topLevels.forEach { item ->
                val count = when (item.route) {
                    DECIDE -> state.decisionQueue.size
                    PIPELINE -> dueFollowUps
                    else -> 0
                }
                item(
                    selected = route == item.route, onClick = { goTo(item.route) }, label = { Text(item.label) },
                    icon = { Icon(if (route == item.route) item.selectedIcon else item.icon, contentDescription = null) },
                    badge = if (count > 0) ({ Badge { Text("$count") } }) else null,
                )
            }
        },
    ) {
        Scaffold(snackbarHost = { SnackbarHost(snackbar) }) { padding ->
            NavHost(navigation, startDestination = TODAY, modifier = Modifier.padding(padding)) {
                composable(TODAY) {
                    ListDetailPage(todayDetails, "Open a job or a company to see it here.", detail = { detail, isAlone -> detailPane(todayDetails, detail, isAlone) }) {
                        TodayScreen(
                            state, onRefresh = viewModel::refresh,
                            onOpenDecisionJob = { todayDetails.show(Detail.job(it, fromQueue = true)) }, onSeeDecide = { goTo(DECIDE) },
                            onOpenCard = { card -> card.detail()?.let(todayDetails::show) }, onSeePipeline = { goTo(PIPELINE) },
                            onFollowedUp = followedUp,
                            onOpenUpdate = { update ->
                                viewModel.markSeen(update)
                                update.detail()?.let(todayDetails::show) ?: navigation.navigate(UPDATES)
                            },
                            onSeeUpdates = { navigation.navigate(UPDATES) },
                            onOpenCompany = { todayDetails.show(Detail.company(it)) },
                            onAskTheMac = { company ->
                                scope.launch {
                                    viewModel.askTheMac(QueueTaskRequest(kind = "research_company", company = company))
                                        .fold({ say("Sent to the Mac. The result comes as an update.") }, { say(it.message ?: "Couldn't reach the Mac.") })
                                }
                            },
                            menu = menu, selected = todayDetails.root,
                        )
                    }
                }
                composable(DECIDE) {
                    ListDetailPage(decideDetails, "Open a job to decide on it here.", detail = { detail, isAlone -> detailPane(decideDetails, detail, isAlone) }) {
                        DecideScreen(
                            state, onRefresh = viewModel::refresh, onOpenJob = { decideDetails.show(Detail.job(it, fromQueue = true)) },
                            menu = menu, selected = decideDetails.root,
                        )
                    }
                }
                composable(PIPELINE) {
                    ListDetailPage(pipelineDetails, "Open a card to see its job or company here.", detail = { detail, isAlone -> detailPane(pipelineDetails, detail, isAlone) }) {
                        val focus by viewModel.pipelineFocus.collectAsStateWithLifecycle()
                        PipelineScreen(
                            state, focus = focus, onFocusShown = viewModel::clearPipelineFocus, onRefresh = viewModel::refresh,
                            onOpenCard = { card -> card.detail()?.let(pipelineDetails::show) }, onFollowedUp = followedUp, onMove = move,
                            menu = menu, selected = pipelineDetails.root,
                        )
                    }
                }
                composable(JOBS) {
                    ListDetailPage(jobsDetails, "Open a job to see it here.", detail = { detail, isAlone -> detailPane(jobsDetails, detail, isAlone) }) {
                        JobsScreen(
                            state, onRefresh = viewModel::refresh, onIncludeUnclear = viewModel::setIncludesUnclear,
                            onOpenJob = { jobsDetails.show(Detail.job(it)) }, menu = menu, selected = jobsDetails.root,
                        )
                    }
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
                // A job or company opened from the updates' history or a notification, over the page.
                composable(JOB, arguments = listOf(navArgument("id") { type = NavType.StringType })) { backStack ->
                    JobScreen(
                        backStack.arguments?.getString("id").orEmpty(), viewModel, onBack = { navigation.popBackStack() },
                        onOpenCompany = { navigation.navigate("company/$it") }, onDecided = { navigation.popBackStack() },
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
