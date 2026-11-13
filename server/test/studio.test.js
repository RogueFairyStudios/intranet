const { createState, applyCommand, exportCredits } = require('../helpers/studio')

describe('studio production records', () => {
  const actor = 8
  function fixture () {
    const state = createState('Game', 'digital', actor)
    applyCommand(state, 'participant', { name: 'Álvaro', roles: ['Art', 'Design'], team: 'Art' }, actor)
    applyCommand(state, 'release', { name: 'Demo' }, actor)
    return state
  }
  test('external and departed contributors retain multiple credit roles', () => {
    const state = fixture()
    const participant = state.participants[0]
    applyCommand(state, 'endParticipation', { id: participant.id }, actor)
    applyCommand(state, 'generateCredits', { releaseId: state.releases[0].id, participantIds: [participant.id] }, actor)
    expect(state.credits[0].manifest.entries.map(e => e.role)).toEqual(['Art', 'Design'])
    expect(state.credits[0].manifest.entries[0].name).toBe('Álvaro')
  })
  test('approved credits remain immutable and exports contain only public fields', () => {
    const state = fixture()
    applyCommand(state, 'generateCredits', { releaseId: state.releases[0].id, participantIds: [state.participants[0].id] }, actor)
    const snapshot = state.credits[0]
    expect(() => exportCredits(snapshot, 'json')).toThrow()
    applyCommand(state, 'approveCredits', { id: snapshot.id }, actor)
    const original = exportCredits(snapshot, 'json')
    state.participants[0].name = 'Changed'
    expect(exportCredits(snapshot, 'json')).toEqual(original)
    expect(original).not.toContain('userId')
    expect(() => applyCommand(state, 'editCredits', { id: snapshot.id, entries: [] }, actor)).toThrow()
  })
  test('generation deduplicates entries and produces stable checksums', () => {
    const state = fixture()
    const input = { releaseId: state.releases[0].id, participantIds: [state.participants[0].id, state.participants[0].id] }
    applyCommand(state, 'generateCredits', input, actor)
    applyCommand(state, 'generateCredits', input, actor)
    expect(state.credits).toHaveLength(1)
    expect(state.credits[0].manifest.entries).toHaveLength(2)
  })
  test('tasks reject invalid owners and record state transitions', () => {
    const state = fixture()
    expect(() => applyCommand(state, 'task', { title: 'Build', assigneeId: 'missing' }, actor)).toThrow()
    applyCommand(state, 'task', { title: 'Build', assigneeId: state.participants[0].id }, actor)
    applyCommand(state, 'transition', { id: state.tasks[0].id, status: 'done' }, actor)
    expect(state.tasks[0].status).toBe('done')
    expect(state.history[state.history.length - 1].action).toBe('transition')
  })
  test('builds require matching approved release credits', () => {
    const state = fixture()
    const releaseId = state.releases[0].id
    applyCommand(state, 'generateCredits', { releaseId, participantIds: [state.participants[0].id] }, actor)
    const snapshot = state.credits[0]
    expect(() => applyCommand(state, 'build', { releaseId, snapshotId: snapshot.id, name: '1' }, actor)).toThrow()
    applyCommand(state, 'approveCredits', { id: snapshot.id }, actor)
    applyCommand(state, 'build', { releaseId, snapshotId: snapshot.id, name: '1' }, actor)
    expect(state.builds[0].checksum).toBe(snapshot.checksum)
    expect(() => applyCommand(state, 'build', { releaseId, snapshotId: snapshot.id, name: '1', uri: 'https://example.org/changed.zip', checksum: 'a'.repeat(64) }, actor)).toThrow('different content')
  })
  test('editorial corrections create a new draft from approved credits', () => {
    const state = fixture()
    applyCommand(state, 'generateCredits', { releaseId: state.releases[0].id, participantIds: [state.participants[0].id] }, actor)
    const approved = state.credits[0]
    applyCommand(state, 'approveCredits', { id: approved.id }, actor)
    const original = exportCredits(approved, 'json')
    applyCommand(state, 'reviseCredits', { id: approved.id }, actor)
    const draft = state.credits[1]
    const entries = JSON.parse(JSON.stringify(draft.manifest.entries))
    entries[0].name = 'Corrected'
    applyCommand(state, 'editCredits', { id: draft.id, entries, reason: 'Name correction' }, actor)
    expect(exportCredits(approved, 'json')).toBe(original)
    expect(draft.manifest.entries[0].name).toBe('Corrected')
  })
})
