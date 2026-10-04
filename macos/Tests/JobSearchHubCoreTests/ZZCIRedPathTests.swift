import Testing

@Test func ciRedPathFailsOnPurpose() {
    #expect(Bool(false), "intentional failure: TP-399 red-path check, this PR is closed unmerged")
}
