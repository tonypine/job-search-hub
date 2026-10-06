import SwiftUI

@MainActor
func drawAll() {
    let directory = URL(filePath: CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : ".")
    let only = CommandLine.arguments.count > 2 ? CommandLine.arguments[2] : nil
    func draw(_ name: String, _ view: some View) {
        if only == nil || only == name { write(view, to: directory, name: name + ".png") }
    }
    draw("patterns", PatternBoard())
    draw("posting-before-after", PostingBeforeAfter())
    draw("job-overview-before-after", OverviewBeforeAfter())
    draw("job-page", JobPageWindow())
    draw("shell-before-after", ShellBeforeAfter())
    draw("pipeline-before-after", PipelineBeforeAfter())
    draw("companies-before-after", CompaniesBeforeAfter())
    draw("decide", DecideWindow())
    draw("criteria-settings-before-after", CriteriaBeforeAfter())
    draw("journeys", JourneysBoard())
    draw("decisions", DecisionsBoard())
    // Last: the rollout shows thumbnails of the images above.
    draw("rollout", RolloutBoard(directory: directory))
}

MainActor.assumeIsolated { drawAll() }
