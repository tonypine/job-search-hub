package com.tonypine.jobsearchhub.core

import kotlin.test.Test
import kotlin.test.fail

class ZZCIRedPathTest {
    @Test
    fun ciRedPathFailsOnPurpose() {
        fail("intentional failure: TP-399 red-path check, this PR is closed unmerged")
    }
}
