<template>
  <section class="studio-pipeline">
    <h3>{{ t('macroPhases') }}</h3>
    <div class="studio-phase-grid">
      <article
        v-for="phase in phases"
        :key="phase"
        :class="['studio-card', 'studio-phase-card', state.phase === phase ? 'is-active' : '']"
        :aria-current="state.phase === phase ? 'step' : null"
      >
        <div class="studio-phase-head">
          <h4>{{ t(phase) }}</h4>
          <span
            v-if="state.phase === phase"
            class="studio-badge"
          >{{ t('projectPhase') }}</span>
        </div>
        <p class="studio-muted">
          {{ t(phase + 'Desc') }}
        </p>
        <p class="studio-muted">
          {{ t('milestones') }}: <strong>{{ milestonesInPhase(phase) }}</strong> ·
          {{ t('openTasks') }}: <strong>{{ tasksInPhase(phase, false) }}</strong> ·
          {{ t('completedTasks') }}: <strong>{{ tasksInPhase(phase, true) }}</strong>
        </p>
        <p
          v-if="phase === 'post_production'"
          class="studio-muted"
        >
          {{ t('distribution') }}: <strong>{{ activeStorefrontsCount }}</strong>
        </p>
        <div>
          <button
            v-if="project.canManage && state.phase !== phase"
            type="button"
            :disabled="busy"
            @click="setProjectPhase(phase)"
          >
            {{ t('setPhase') }}
          </button>
          <button
            v-if="phase === 'post_production'"
            type="button"
            @click="$emit('open-section', 'distribution')"
          >
            {{ t('distribution') }}
          </button>
        </div>
      </article>
    </div>

    <div class="studio-section-head">
      <div>
        <h3>{{ t('assetPipelineMatrix') }}</h3>
        <p class="studio-muted">
          {{ t('assetPipelineHint') }}
        </p>
      </div>
    </div>

    <details
      v-if="project.canEdit"
      :open="!!artifactForm.id"
      class="studio-settings"
    >
      <summary>{{ artifactForm.id ? t('edit') + ' · ' + artifactForm.name : t('newArtifact') }}</summary>
      <form
        class="studio-inline-form"
        @submit.prevent="saveArtifact"
      >
        <div class="studio-form-grid">
          <label>{{ t('name') }}<input
            v-model="artifactForm.name"
            required
            maxlength="500"
          ></label>
          <label>{{ t('kind') }}<input
            v-model="artifactForm.kind"
            required
            maxlength="120"
            placeholder="dialogue, sprite, scene, spirit, map…"
          ></label>
          <label>{{ t('track') }}<select v-model="artifactForm.track"><option
            v-for="item in tracks"
            :key="item"
            :value="item"
          >{{ t(item) }}</option></select></label>
          <label>{{ t('stage') }}<select v-model="artifactForm.stage"><option
            v-for="item in stages"
            :key="item"
            :value="item"
          >{{ t(item) }}</option></select></label>
          <label>{{ t('tool') }}<select v-model="artifactForm.tool"><option
            v-for="item in tools"
            :key="item"
            :value="item"
          >{{ t(item) }}</option></select></label>
          <label>{{ t('repoId') }}<select v-model="artifactForm.repoId"><option value="">{{ t('none') }}</option><option
            v-for="repo in state.repositories"
            :key="repo.id"
            :value="repo.id"
          >{{ repo.name }} ({{ repo.slug }})</option></select></label>
          <label>{{ t('assetPath') }}<input
            v-model="artifactForm.assetPath"
            maxlength="1000"
            placeholder="Content/Dialogues/intro.yaml"
          ></label>
          <label>{{ t('uriTarget') }}<input
            v-model="artifactForm.uriTarget"
            maxlength="200"
            placeholder="start_node, e1m1, BaseLayer…"
          ></label>
          <label>{{ t('taskId') }}<select v-model="artifactForm.taskId"><option value="">{{ t('none') }}</option><option
            v-for="item in state.tasks"
            :key="item.id"
            :value="item.id"
          >{{ item.title }}</option></select></label>
        </div>
        <label>{{ t('toolUri') }}<input
          v-model="artifactForm.toolUri"
          maxlength="4000"
          placeholder="goethe://open?repo=…&path=…"
        ></label>
        <div>
          <button
            type="button"
            :disabled="busy"
            @click="composeUri"
          >
            {{ t('composeUri') }}
          </button>
          <button :disabled="busy">
            {{ t('save') }}
          </button>
          <button
            v-if="artifactForm.id"
            type="button"
            :disabled="busy"
            @click="resetArtifactForm"
          >
            {{ t('cancel') }}
          </button>
        </div>
      </form>
    </details>

    <div class="studio-filters">
      <label>{{ t('search') }}<input
        v-model="artifactFilter"
        type="search"
      ></label>
      <label>{{ t('track') }}<select v-model="trackFilter"><option value="">{{ t('all') }}</option><option
        v-for="item in tracks"
        :key="item"
        :value="item"
      >{{ t(item) }}</option></select></label>
      <label>{{ t('tool') }}<select v-model="toolFilter"><option value="">{{ t('all') }}</option><option
        v-for="item in tools"
        :key="item"
        :value="item"
      >{{ t(item) }}</option></select></label>
    </div>

    <p
      v-if="!filteredArtifacts.length"
      class="studio-muted"
    >
      {{ t('noItems') }}
    </p>
    <div
      v-else
      class="studio-matrix-wrap"
    >
      <table class="studio-matrix">
        <thead>
          <tr>
            <th>{{ t('artifacts') }}</th>
            <th
              v-for="stg in stages"
              :key="stg"
            >
              {{ t(stg) }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="item in filteredArtifacts"
            :key="item.id"
          >
            <td class="studio-matrix-asset">
              <div class="studio-asset-header">
                <strong>{{ item.name }}</strong>
                <span class="studio-badge">{{ t(item.track || 'general') }}</span>
                <span class="studio-badge">{{ t(item.tool || 'generic') }}</span>
              </div>
              <p
                v-if="item.assetPath || item.repoId"
                class="studio-muted checksum"
              >
                {{ repoLabel(item.repoId) }}{{ item.assetPath ? (item.repoId ? ':' : '') + item.assetPath : '' }}
                <span v-if="item.lastVcsRevision"> @ {{ item.lastVcsRevision.slice(0, 12) }}</span>
              </p>
              <div
                v-if="vcsBadgeFor(item).lock || vcsBadgeFor(item).modified"
                class="studio-vcs-flags"
              >
                <span
                  v-if="vcsBadgeFor(item).lock"
                  class="studio-badge studio-lock-badge"
                >{{ t('lockedBy') }}: {{ vcsBadgeFor(item).lock.owner }}</span>
                <span
                  v-if="vcsBadgeFor(item).modified"
                  class="studio-badge studio-dirty-badge"
                >{{ t('modifiedLocal') }}</span>
              </div>
              <div v-if="item.toolUri">
                <uri-link
                  :uri="item.toolUri"
                  :t="t"
                  compact
                />
              </div>
              <div
                v-if="latestVersion(item.id)"
                class="studio-muted"
              >
                <span>v{{ latestVersion(item.id).version }} · {{ t(versionDecision(latestVersion(item.id).id)) }}</span>
                <uri-link
                  :uri="latestVersion(item.id).uri"
                  :t="t"
                  compact
                />
              </div>
              <div v-if="project.canEdit">
                <button
                  type="button"
                  class="studio-btn-inline"
                  :disabled="busy"
                  @click="editArtifact(item)"
                >
                  {{ t('edit') }}
                </button>
              </div>
            </td>
            <td
              v-for="(stg, idx) in stages"
              :key="stg"
              class="studio-stage-td"
            >
              <button
                type="button"
                :class="['studio-stage-cell', stageStateClass(item, idx)]"
                :aria-pressed="(item.stage || 'concept') === stg"
                :disabled="busy || !project.canEdit"
                @click="moveStage(item, stg)"
              >
                <span class="studio-stage-dot">{{ stageSymbol(item, idx) }}</span>
                <span class="studio-stage-label">{{ t(stg) }}</span>
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <h3>{{ t('repositories') }}</h3>
    <details
      v-if="project.canEdit"
      :open="!!repoForm.id"
      class="studio-settings"
    >
      <summary>{{ repoForm.id ? t('edit') + ' · ' + repoForm.name : t('newRepository') }}</summary>
      <form
        class="studio-inline-form"
        @submit.prevent="saveRepository"
      >
        <div class="studio-form-grid">
          <label>{{ t('name') }}<input
            v-model="repoForm.name"
            required
            maxlength="200"
          ></label>
          <label>{{ t('repositorySlug') }}<input
            v-model="repoForm.slug"
            maxlength="120"
            placeholder="ghostlore-assets"
          ></label>
          <label>{{ t('vcsSystem') }}<select v-model="repoForm.system"><option value="git">{{ t('git') }}</option><option value="lore">{{ t('lore') }}</option><option value="urc">{{ t('urc') }}</option></select></label>
          <label>{{ t('defaultBranch') }}<input
            v-model="repoForm.defaultBranch"
            maxlength="200"
            placeholder="main"
          ></label>
          <label>{{ t('remoteUri') }}<input
            v-model="repoForm.remoteUri"
            maxlength="2000"
            placeholder="lores://lore.roguefairy.net:41337/project or https://github.com/…"
          ></label>
          <label>{{ t('workspacePath') }}<input
            v-model="repoForm.workspacePath"
            maxlength="1000"
            placeholder="games/ghostlore"
          ></label>
        </div>
        <button :disabled="busy">
          {{ t('save') }}
        </button>
        <button
          v-if="repoForm.id"
          type="button"
          :disabled="busy"
          @click="resetRepoForm"
        >
          {{ t('cancel') }}
        </button>
      </form>
    </details>

    <details
      v-if="project.canEdit && state.repositories.length"
      class="studio-settings"
    >
      <summary>{{ t('reportVcsStatus') }}</summary>
      <form
        class="studio-inline-form"
        @submit.prevent="saveVcsStatus"
      >
        <div class="studio-form-grid">
          <label>{{ t('repoId') }}<select
            v-model="statusForm.repositoryId"
            required
          ><option
            v-for="repo in state.repositories"
            :key="repo.id"
            :value="repo.id"
          >{{ repo.name }} ({{ repo.slug }})</option></select></label>
          <label>{{ t('branch') }}<input
            v-model="statusForm.branch"
            maxlength="200"
            placeholder="main"
          ></label>
          <label>{{ t('revision') }}<input
            v-model="statusForm.revision"
            maxlength="200"
            placeholder="SHA or branch@42"
          ></label>
          <label>{{ t('author') }}<input
            v-model="statusForm.author"
            maxlength="200"
          ></label>
          <label>{{ t('commitMessage') }}<input
            v-model="statusForm.commitMessage"
            maxlength="1000"
          ></label>
          <label>{{ t('modifiedCount') }}<input
            v-model.number="statusForm.modifiedCount"
            type="number"
            min="0"
          ></label>
        </div>
        <button :disabled="busy">
          {{ t('save') }}
        </button>
      </form>
    </details>

    <p
      v-if="!state.repositories.length"
      class="studio-muted"
    >
      {{ t('noItems') }}
    </p>
    <div class="studio-repo-grid">
      <article
        v-for="repo in state.repositories"
        :key="repo.id"
        class="studio-card"
      >
        <div class="studio-asset-header">
          <h4>{{ repo.name }}</h4>
          <span class="studio-badge">{{ t(repo.system) }}</span>
          <span class="studio-badge checksum">{{ repo.slug }}</span>
        </div>
        <div v-if="repo.remoteUri">
          <uri-link
            :uri="repo.remoteUri"
            :t="t"
          />
        </div>
        <div v-if="repo.status">
          <p>
            <span :class="['studio-badge', repo.status.clean ? 'studio-clean-badge' : 'studio-dirty-badge']">
              {{ repo.status.clean ? t('cleanTree') : t('dirtyTree') + ' (' + repo.status.modifiedCount + ')' }}
            </span>
            <span
              v-if="repo.status.compression"
              class="studio-badge checksum"
            >{{ repo.status.compression }}</span>
          </p>
          <p class="checksum">
            <strong>{{ repo.status.branch }}</strong>
            <span v-if="repo.status.revision"> · {{ shortRevision(repo.status) }}</span>
            <span v-if="repo.status.author"> · {{ repo.status.author }}</span>
          </p>
          <p
            v-if="repo.status.commitMessage"
            class="studio-muted"
          >
            {{ repo.status.commitMessage }}
          </p>
          <div
            v-if="repo.status.locks && repo.status.locks.length"
            class="studio-lock-list"
          >
            <p class="studio-muted">
              <strong>{{ t('activeLocks') }} ({{ repo.status.locks.length }})</strong>
            </p>
            <p
              v-for="(lock, lIdx) in repo.status.locks"
              :key="lIdx"
              class="checksum"
            >
              <span class="studio-badge studio-lock-badge">{{ lock.owner }}</span>
              {{ lock.path }}
              <span
                v-if="lock.branch"
                class="studio-muted"
              >({{ lock.branch }})</span>
            </p>
          </div>
        </div>
        <p
          v-else
          class="studio-muted"
        >
          {{ t('noStatusYet') }}
        </p>
        <p class="studio-muted checksum">
          {{ t('cliSyncHint') }}: python integrations/cli/studio_client.py vcs-sync "{{ repo.workspacePath || '.' }}" --slug {{ repo.slug }}
        </p>
        <button
          v-if="project.canEdit"
          type="button"
          :disabled="busy"
          @click="editRepository(repo)"
        >
          {{ t('edit') }}
        </button>
      </article>
    </div>
  </section>
</template>
<script>
import UriLink from './uri-link.vue'

const phases = ['pre_production', 'production', 'post_production', 'live']
const stages = ['concept', 'blockout', 'authoring', 'surface_loc', 'engine_integration', 'polish']
const tracks = ['general', 'art_2d', 'art_3d', 'narrative', 'spirit', 'scene_level', 'audio_vfx', 'code_system', 'typography', 'tabletop_print']
const tools = ['generic', 'goethe', 'geppetto', 'godot', 'carnifex', 'animafaber', 'fontpit', 'lore', 'urc', 'uefn', 'unity', 'git', 'github']

function emptyArtifact () {
  return {
    id: '',
    name: '',
    kind: '',
    track: 'general',
    stage: 'concept',
    tool: 'generic',
    repoId: '',
    assetPath: '',
    uriTarget: '',
    toolUri: '',
    taskId: ''
  }
}

function emptyRepo () {
  return {
    id: '',
    name: '',
    slug: '',
    system: 'git',
    remoteUri: '',
    defaultBranch: 'main',
    workspacePath: ''
  }
}

export default {
  components: { UriLink },
  props: {
    project: { type: Object, required: true },
    busy: Boolean,
    t: { type: Function, required: true },
    command: { type: Function, required: true }
  },
  data () {
    return {
      phases,
      stages,
      tracks,
      tools,
      artifactFilter: '',
      trackFilter: '',
      toolFilter: '',
      artifactForm: emptyArtifact(),
      repoForm: emptyRepo(),
      statusForm: {
        repositoryId: '',
        branch: 'main',
        revision: '',
        author: '',
        commitMessage: '',
        modifiedCount: 0
      }
    }
  },
  computed: {
    state () {
      return this.project.data
    },
    activeStorefrontsCount () {
      return (this.state.platformTargets || []).filter(t => !t.archived).length
    },
    filteredArtifacts () {
      const needle = this.artifactFilter.trim().toLowerCase()
      return this.state.artifacts.filter(item => {
        if (this.trackFilter && (item.track || 'general') !== this.trackFilter) return false
        if (this.toolFilter && (item.tool || 'generic') !== this.toolFilter) return false
        if (!needle) return true
        const hay = [item.name, item.kind, item.assetPath, item.toolUri].filter(Boolean).join(' ').toLowerCase()
        return hay.includes(needle)
      })
    }
  },
  watch: {
    'state.repositories': {
      immediate: true,
      handler (list) {
        if (list && list.length && !this.statusForm.repositoryId) {
          this.statusForm.repositoryId = list[0].id
        }
      }
    }
  },
  methods: {
    milestonesInPhase (phase) {
      return this.state.milestones.filter(m => (m.phase || 'pre_production') === phase).length
    },
    tasksInPhase (phase, completed) {
      const milestonePhase = new Map(this.state.milestones.map(m => [m.id, m.phase || 'pre_production']))
      return this.state.tasks.filter(task => {
        const resolved = task.phase || milestonePhase.get(task.milestoneId) || this.state.phase || 'pre_production'
        return resolved === phase && (completed ? task.status === 'done' : task.status !== 'done')
      }).length
    },
    setProjectPhase (phase) {
      return this.command('project', {
        name: this.state.name,
        kind: this.state.kind,
        archived: !!this.state.archived,
        phase
      })
    },
    slugify (raw) {
      return String(raw || 'project').toLowerCase().replace(/[^a-z0-9._-]+/g, '-').replace(/^-+|-+$/g, '') || 'project'
    },
    composeUri () {
      const repo = this.state.repositories.find(r => r.id === this.artifactForm.repoId)
      const repoKey = repo ? (repo.workspacePath || repo.slug) : this.slugify(this.state.name)
      const path = (this.artifactForm.assetPath || '').trim().replace(/\\/g, '/').replace(/^\/+/, '')
      const target = (this.artifactForm.uriTarget || '').trim()
      const enc = encodeURIComponent
      const tool = this.artifactForm.tool
      let built = ''
      if (tool === 'goethe') {
        built = `goethe://open?repo=${enc(repoKey)}${path ? '&path=' + enc(path) : ''}${target ? '&node=' + enc(target) : ''}`
      } else if (tool === 'geppetto') {
        built = `geppetto://open?repo=${enc(repoKey)}${path ? '&path=' + enc(path) : ''}${target ? '&layer=' + enc(target) : ''}`
      } else if (tool === 'godot') {
        const resPath = path ? (path.startsWith('res://') ? path : 'res://' + path) : ''
        built = `godot://open?project=${enc(repoKey)}${resPath ? '&scene=' + enc(resPath) : ''}${target ? '&node=' + enc(target) : ''}`
      } else if (tool === 'carnifex') {
        const mapName = target || path
        built = `carnifex://open?game=${enc(repoKey)}${mapName ? '&map=' + enc(mapName) : ''}`
      } else if (tool === 'lore' || tool === 'urc') {
        built = `lore://open?repo=${enc(repoKey)}${path ? '&path=' + enc(path) : ''}`
      } else if (tool === 'uefn') {
        built = `uefn://open?project=${enc(repoKey)}${path ? '&path=' + enc(path) : ''}`
      } else if (tool === 'animafaber') {
        built = `animafaber://open?spirit=${enc(target || path || this.slugify(this.artifactForm.name))}`
      } else if (tool === 'fontpit') {
        built = `fontpit://open?family=${enc(target || path || this.slugify(this.artifactForm.name))}`
      }
      if (built) this.artifactForm.toolUri = built
    },
    editArtifact (item) {
      this.artifactForm = {
        id: item.id,
        name: item.name || '',
        kind: item.kind || '',
        track: item.track || 'general',
        stage: item.stage || 'concept',
        tool: item.tool || 'generic',
        repoId: item.repoId || '',
        assetPath: item.assetPath || '',
        uriTarget: '',
        toolUri: item.toolUri || '',
        taskId: item.taskId || ''
      }
    },
    resetArtifactForm () {
      this.artifactForm = emptyArtifact()
    },
    async saveArtifact () {
      const payload = {
        name: this.artifactForm.name,
        kind: this.artifactForm.kind,
        track: this.artifactForm.track,
        stage: this.artifactForm.stage,
        tool: this.artifactForm.tool,
        repoId: this.artifactForm.repoId || null,
        assetPath: this.artifactForm.assetPath || '',
        toolUri: this.artifactForm.toolUri || '',
        taskId: this.artifactForm.taskId || null
      }
      if (this.artifactForm.id) payload.id = this.artifactForm.id
      if (await this.command('artifact', payload)) {
        this.resetArtifactForm()
      }
    },
    moveStage (item, stg) {
      if ((item.stage || 'concept') === stg) return
      return this.command('artifactStage', { id: item.id, stage: stg })
    },
    stageStateClass (item, idx) {
      const currentIdx = stages.indexOf(item.stage || 'concept')
      if (idx === currentIdx) return 'is-current'
      if (idx < currentIdx) return 'is-passed'
      return 'is-upcoming'
    },
    stageSymbol (item, idx) {
      const currentIdx = stages.indexOf(item.stage || 'concept')
      if (idx === currentIdx) return '●'
      if (idx < currentIdx) return '✓'
      return '○'
    },
    repoLabel (repoId) {
      const repo = this.state.repositories.find(r => r.id === repoId)
      return repo ? repo.slug : ''
    },
    vcsBadgeFor (item) {
      if (!item.assetPath) return { lock: null, modified: false }
      const norm = item.assetPath.replace(/\\/g, '/').replace(/^\/+/, '')
      const repos = item.repoId ?
        this.state.repositories.filter(r => r.id === item.repoId) :
        this.state.repositories
      for (const repo of repos) {
        if (!repo.status) continue
        const lock = (repo.status.locks || []).find(l => l.path === norm || norm.startsWith(l.path + '/') || l.path.startsWith(norm + '/'))
        const modified = (repo.status.modifiedFiles || []).some(f => f === norm || f.startsWith(norm + '/'))
        if (lock || modified) return { lock: lock || null, modified }
      }
      return { lock: null, modified: false }
    },
    latestVersion (artifactId) {
      const versions = this.state.artifactVersions.filter(v => v.artifactId === artifactId)
      return versions.length ? versions[versions.length - 1] : null
    },
    versionDecision (versionId) {
      const reviews = this.state.artifactReviews.filter(r => r.versionId === versionId)
      return reviews.length ? reviews[reviews.length - 1].decision : 'pending'
    },
    editRepository (repo) {
      this.repoForm = {
        id: repo.id,
        name: repo.name,
        slug: repo.slug,
        system: repo.system,
        remoteUri: repo.remoteUri || '',
        defaultBranch: repo.defaultBranch || 'main',
        workspacePath: repo.workspacePath || ''
      }
    },
    resetRepoForm () {
      this.repoForm = emptyRepo()
    },
    async saveRepository () {
      const payload = { ...this.repoForm }
      if (!payload.id) delete payload.id
      if (await this.command('repository', payload)) {
        this.resetRepoForm()
      }
    },
    async saveVcsStatus () {
      const modCount = Number(this.statusForm.modifiedCount) || 0
      const payload = {
        repositoryId: this.statusForm.repositoryId,
        branch: this.statusForm.branch || 'main',
        revision: this.statusForm.revision,
        author: this.statusForm.author,
        commitMessage: this.statusForm.commitMessage,
        modifiedCount: modCount,
        clean: modCount === 0
      }
      if (await this.command('vcsStatus', payload)) {
        this.statusForm.revision = ''
        this.statusForm.commitMessage = ''
      }
    },
    shortRevision (status) {
      if (status.revisionNumber !== null && status.revisionNumber !== undefined) {
        return '@' + status.revisionNumber + (status.revision ? ' (' + status.revision.slice(0, 12) + ')' : '')
      }
      return status.revision.length > 16 ? status.revision.slice(0, 12) : status.revision
    }
  }
}
</script>
