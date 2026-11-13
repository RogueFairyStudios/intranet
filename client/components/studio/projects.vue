<template>
  <v-app :dark="$vuetify.theme.dark">
    <nav-header />
    <v-main>
      <section class="studio">
        <header class="studio-heading">
          <h1>{{ t('projects') }}</h1>
          <button
            :disabled="busy"
            @click="load"
          >
            {{ t('reload') }}
          </button>
        </header>
        <p
          v-if="error"
          role="alert"
          class="studio-error"
        >
          {{ error }}
        </p>
        <p
          v-if="busy"
          role="status"
          class="studio-muted"
        >
          {{ t('loading') }}
        </p>
        <div class="studio-layout">
          <aside>
            <button
              v-for="item in projects"
              :key="item.id"
              class="project-button"
              :disabled="busy"
              :aria-pressed="selected === item.id"
              @click="select(item.id)"
            >
              <span>{{ item.data.name }}</span>
              <small class="project-meta">{{ t(item.data.kind) }}{{ item.data.archived ? ' · ' + t('archived') : '' }}</small>
            </button>
            <p
              v-if="!projects.length && !busy"
              class="studio-muted"
            >
              {{ t('empty') }}
            </p>
            <form
              v-if="canCreateProject"
              @submit.prevent="createProject"
            >
              <h2>{{ t('newProject') }}</h2>
              <label>{{ t('name') }}<input
                v-model="newProject.name"
                required
                maxlength="500"
              ></label>
              <label>{{ t('type') }}<select v-model="newProject.kind"><option value="digital">{{ t('digital') }}</option><option value="analogue">{{ t('analogue') }}</option><option value="hybrid">{{ t('hybrid') }}</option></select></label>
              <button :disabled="busy">
                {{ t('create') }}
              </button>
            </form>
          </aside>
          <section
            v-if="project"
            :key="selected"
            class="project-content"
          >
            <header class="project-title-row">
              <h2>
                {{ project.data.name }}
                <small class="studio-badge">{{ t(project.data.kind) }}</small>
                <small
                  v-if="project.data.archived"
                  class="studio-badge"
                >{{ t('archived') }}</small>
              </h2>
            </header>
            <details
              v-if="project.canManage"
              class="studio-settings"
            >
              <summary>{{ t('editProject') }}</summary>
              <record-form
                :key="project.id + project.revision"
                :title="t('editProject')"
                :fields="projectFields"
                :value="project.data"
                :t="t"
                :busy="busy"
                :submit="saveProject"
              />
              <button
                :disabled="busy"
                @click="command('project', { name: project.data.name, kind: project.data.kind, archived: !project.data.archived })"
              >
                {{ t(project.data.archived ? 'restore' : 'archive') }}
              </button>
            </details>
            <nav
              class="studio-tabs"
              :aria-label="t('sections')"
            >
              <button
                v-for="section in sections"
                :key="section"
                :aria-pressed="tab === section"
                @click="setTab(section)"
              >
                {{ t(section) }}
              </button>
            </nav>
            <section v-if="tab === 'overview'">
              <div class="studio-metrics">
                <article class="studio-card">
                  <h3>{{ t('activeMilestone') }}</h3>
                  <p><strong>{{ currentMilestone ? currentMilestone.name : t('none') }}</strong></p>
                  <p
                    v-if="currentMilestone && currentMilestone.dueDate"
                    class="studio-muted"
                  >
                    {{ t('dueDate') }}: {{ currentMilestone.dueDate.slice(0, 10) }}
                  </p>
                </article>
                <article class="studio-card">
                  <h3>{{ t('activeCycle') }}</h3>
                  <p><strong>{{ currentCycle ? currentCycle.name : t('none') }}</strong></p>
                  <p
                    v-if="currentCycle && (currentCycle.startDate || currentCycle.endDate)"
                    class="studio-muted"
                  >
                    {{ (currentCycle.startDate || '').slice(0, 10) }} — {{ (currentCycle.endDate || '').slice(0, 10) }}
                  </p>
                </article>
                <article class="studio-card">
                  <h3>{{ t('openTasks') }}</h3>
                  <p><strong>{{ openTasks.length }}</strong> · {{ t('completedTasks') }}: {{ doneTasksCount }}</p>
                </article>
                <article class="studio-card">
                  <h3>{{ t('activeAssignees') }}</h3>
                  <p><strong>{{ activePeople.length }}</strong></p>
                  <p
                    v-if="activePeople.length"
                    class="studio-muted"
                  >
                    {{ activePeople.slice(0, 5).map(person => person.name).join(', ') }}
                  </p>
                </article>
              </div>
              <div class="studio-overview-grid">
                <article class="studio-card">
                  <h3>{{ t('blockers') }}</h3>
                  <p
                    v-if="!blockedTasks.length"
                    class="studio-muted"
                  >
                    {{ t('none') }}
                  </p>
                  <p
                    v-for="item in blockedTasks"
                    :key="item.id"
                  >
                    <strong>{{ item.title }}</strong> · {{ personName(item.assigneeId) }}
                    <span
                      v-if="item.blockedReason"
                      class="studio-muted"
                    > — {{ item.blockedReason }}</span>
                  </p>
                  <button
                    type="button"
                    @click="setTab('work')"
                  >
                    {{ t('work') }}
                  </button>
                </article>
                <article class="studio-card">
                  <h3>{{ t('pendingReviews') }}</h3>
                  <p
                    v-if="!pendingArtifactVersions.length"
                    class="studio-muted"
                  >
                    {{ t('none') }}
                  </p>
                  <p
                    v-for="ver in pendingArtifactVersions"
                    :key="ver.id"
                  >
                    {{ artifactVersionLabel(ver) }}
                  </p>
                  <button
                    type="button"
                    @click="setTab('production')"
                  >
                    {{ t('production') }}
                  </button>
                </article>
                <article class="studio-card">
                  <h3>{{ t('outdatedDocs') }}</h3>
                  <p
                    v-if="!outdatedDocuments.length"
                    class="studio-muted"
                  >
                    {{ t('none') }}
                  </p>
                  <p
                    v-for="doc in outdatedDocuments"
                    :key="doc.id"
                  >
                    <a :href="pageUrl(doc)">{{ doc.title }}</a>
                    <button
                      v-if="project.canEdit"
                      :disabled="busy"
                      @click="command('refreshDocument', { id: doc.id })"
                    >
                      {{ t('refreshReference') }}
                    </button>
                  </p>
                  <button
                    type="button"
                    @click="setTab('documents')"
                  >
                    {{ t('documents') }}
                  </button>
                </article>
                <article class="studio-card">
                  <h3>{{ t('latestReleases') }}</h3>
                  <p
                    v-if="!recentReleases.length"
                    class="studio-muted"
                  >
                    {{ t('noItems') }}
                  </p>
                  <p
                    v-for="rel in recentReleases"
                    :key="rel.id"
                  >
                    <strong>{{ rel.name }}</strong> · {{ rel.publishedAt ? t('published') + ' (' + rel.publishedAt.slice(0, 10) + ')' : t('draft') }}
                  </p>
                  <button
                    type="button"
                    @click="setTab('releases')"
                  >
                    {{ t('releases') }}
                  </button>
                </article>
              </div>
            </section>
            <delivery-panel
              v-if="['planning', 'production', 'validation', 'releases'].includes(tab)"
              :key="project.id + tab"
              :project="project"
              :section="tab"
              :t="t"
              :busy="busy"
              :command="command"
            />
            <section v-if="tab === 'team'">
              <form
                v-if="project.canEdit"
                @submit.prevent="saveTeam"
              >
                <h3>{{ t('teams') }}</h3>
                <label>{{ t('name') }}<input
                  v-model="team.name"
                  required
                  maxlength="120"
                ></label>
                <button :disabled="busy">
                  {{ t('save') }}
                </button>
                <button
                  type="button"
                  @click="team = { name: '' }"
                >
                  {{ t('cancel') }}
                </button>
              </form>
              <article
                v-for="group in project.data.teams"
                :key="group.id"
                class="studio-card"
              >
                <h4>{{ group.name }} {{ group.archived ? '· ' + t('archived') : '' }}</h4>
                <button
                  v-if="project.canEdit"
                  :disabled="busy"
                  @click="team = { ...group }"
                >
                  {{ t('edit') }}
                </button>
                <button
                  v-if="project.canEdit"
                  :disabled="busy"
                  @click="command('team', { ...group, archived: !group.archived })"
                >
                  {{ t(group.archived ? 'restore' : 'archive') }}
                </button>
              </article>
              <form
                v-if="project.canEdit"
                @submit.prevent="savePerson"
              >
                <h3>{{ person.id ? t('edit') : t('addParticipant') }}</h3>
                <label>{{ t('publicName') }}<input
                  v-model="person.name"
                  required
                  maxlength="255"
                ></label>
                <label>{{ t('teamName') }}<select v-model="person.teamId"><option value="">{{ t('none') }}</option><option
                  v-for="group in project.data.teams.filter(row => !row.archived || row.id === person.teamId)"
                  :key="group.id"
                  :value="group.id"
                >{{ group.name }}</option></select></label>
                <search-picker
                  v-model="person.userId"
                  :project-id="selected"
                  kind="users"
                  :label="t('linkedUser')"
                  :t="t"
                />
                <label>{{ t('roles') }}<input
                  v-model="person.roleText"
                  required
                ></label>
                <label>{{ t('order') }}<input
                  v-model.number="person.order"
                  type="number"
                  step="1"
                ></label>
                <button :disabled="busy">
                  {{ t('save') }}
                </button>
                <button
                  v-if="person.id"
                  type="button"
                  @click="person = { name: '', teamId: '', userId: null, roleText: '', order: 0 }"
                >
                  {{ t('cancel') }}
                </button>
              </form>
              <article
                v-for="member in project.data.participants"
                :key="member.id"
                class="studio-card"
              >
                <h3>{{ member.name }}</h3>
                <p>{{ member.team ? member.team + ' · ' : '' }}{{ member.roles.join(', ') }}</p>
                <p class="studio-muted">
                  {{ member.startedAt.slice(0, 10) }} · {{ member.endedAt ? t('ended') + ' ' + member.endedAt.slice(0, 10) : t('active') }}
                </p>
                <button
                  v-if="project.canEdit"
                  :disabled="busy"
                  @click="editPerson(member)"
                >
                  {{ t('edit') }}
                </button>
                <button
                  v-if="project.canEdit && !member.endedAt"
                  :disabled="busy"
                  @click="command('endParticipation', { id: member.id })"
                >
                  {{ t('endParticipation') }}
                </button>
              </article>
              <form
                v-if="project.canManage"
                @submit.prevent="command('access', access)"
              >
                <h3>{{ t('access') }}</h3>
                <search-picker
                  v-model="access.userId"
                  :project-id="selected"
                  kind="users"
                  :label="t('linkedUser')"
                  :t="t"
                  required
                />
                <label>{{ t('accessRole') }}<select v-model="access.role"><option
                  v-for="role in accessRoles"
                  :key="role"
                  :value="role"
                >{{ t(role) }}</option></select></label>
                <button :disabled="busy">
                  {{ t('save') }}
                </button>
                <p
                  v-for="entry in project.data.access"
                  :key="entry.userId"
                >
                  {{ entry.name || ('#' + entry.userId) }} · {{ t(entry.role) }}
                </p>
              </form>
              <form
                v-if="project.canManage"
                @submit.prevent="command('integrationAccess', integration)"
              >
                <h3>{{ t('integrations') }}</h3>
                <search-picker
                  v-model="integration.groupId"
                  :project-id="selected"
                  kind="groups"
                  :label="t('group')"
                  :t="t"
                  required
                />
                <label>{{ t('accessRole') }}<select v-model="integration.role"><option
                  v-for="role in ['reader', 'editor', 'remove']"
                  :key="role"
                  :value="role"
                >{{ t(role) }}</option></select></label>
                <button :disabled="busy">
                  {{ t('save') }}
                </button>
                <p
                  v-for="entry in project.data.integrationAccess"
                  :key="entry.groupId"
                >
                  {{ entry.name || (t('group') + ' #' + entry.groupId) }} · {{ t(entry.role) }}
                </p>
              </form>
            </section>
            <section v-if="tab === 'documents'">
              <form
                v-if="project.canEdit"
                @submit.prevent="command('createDocument', newDocument)"
              >
                <h3>{{ t('newDocument') }}</h3>
                <label>{{ t('template') }}<select
                  v-model="newDocument.template"
                  required
                ><option
                  v-for="template in templates"
                  :key="template.key"
                  :value="template.key"
                >{{ template.title }}</option></select></label>
                <label>{{ t('title') }}<input
                  v-model="newDocument.title"
                  required
                  maxlength="255"
                ></label>
                <label>{{ t('pagePath') }}<input
                  v-model="newDocument.path"
                  required
                  maxlength="500"
                  placeholder="games/project/gdd"
                ></label>
                <label>{{ t('locale') }}<input
                  v-model="newDocument.locale"
                  required
                  maxlength="10"
                ></label>
                <button :disabled="busy">
                  {{ t('createDraft') }}
                </button>
              </form>
              <form
                v-if="project.canEdit"
                @submit.prevent="command('document', document)"
              >
                <search-picker
                  v-model="document.pageId"
                  :project-id="selected"
                  kind="pages"
                  :label="t('documents')"
                  :t="t"
                  required
                />
                <button :disabled="busy">
                  {{ t('link') }}
                </button>
              </form>
              <p
                v-if="!project.data.documents.length"
                class="studio-muted"
              >
                {{ t('noItems') }}
              </p>
              <article
                v-for="doc in project.data.documents"
                :key="doc.id"
                class="studio-card"
              >
                <h4><a :href="pageUrl(doc)">{{ doc.title }}</a></h4>
                <p class="studio-muted">
                  /{{ doc.locale }}/{{ doc.path }} · {{ doc.revision ? doc.revision.slice(0, 19).replace('T', ' ') : '' }}
                  <span v-if="doc.outdated"> · {{ t('documentChanged') }}</span>
                </p>
                <button
                  v-if="doc.outdated && project.canEdit"
                  :disabled="busy"
                  @click="command('refreshDocument', { id: doc.id })"
                >
                  {{ t('refreshReference') }}
                </button>
              </article>
            </section>
            <section v-if="tab === 'work'">
              <record-form
                v-if="project.canEdit"
                :title="task.id ? t('edit') : t('task')"
                :fields="taskFields"
                :value="task"
                :t="t"
                :busy="busy"
                :submit="saveTask"
                @cancel="resetTask"
              />
              <div class="studio-filters">
                <label>{{ t('search') }}<input
                  v-model="taskFilter"
                  type="search"
                ></label>
                <label>{{ t('assignee') }}<select v-model="assigneeFilter"><option value="">{{ t('all') }}</option><option
                  v-for="member in project.data.participants"
                  :key="member.id"
                  :value="member.id"
                >{{ member.name }}</option></select></label>
                <label>{{ t('view') }}<select v-model="view"><option value="board">{{ t('board') }}</option><option value="list">{{ t('list') }}</option></select></label>
              </div>
              <div :class="view === 'board' ? 'studio-board' : ''">
                <section
                  v-for="status in statuses"
                  :key="status"
                  class="studio-column"
                >
                  <h3>{{ t(status) }}</h3>
                  <p
                    v-if="!tasksFor(status).length"
                    class="studio-muted"
                  >
                    {{ t('noItems') }}
                  </p>
                  <article
                    v-for="item in tasksFor(status)"
                    :key="item.id"
                    class="studio-card"
                  >
                    <h4>{{ item.title }}</h4>
                    <p v-if="item.criteria">
                      {{ item.criteria }}
                    </p>
                    <p class="studio-muted">
                      {{ item.discipline ? item.discipline + ' · ' : '' }}{{ personName(item.assigneeId) }}
                    </p>
                    <p class="studio-muted">
                      {{ t(item.priority || 'normal') }}{{ item.dueDate ? ' · ' + item.dueDate.slice(0, 10) : '' }}{{ item.blockedReason ? ' · ' + item.blockedReason : '' }}
                    </p>
                    <p
                      v-for="doc in project.data.documents.filter(row => (item.documentIds || []).includes(row.id))"
                      :key="doc.id"
                    >
                      <a :href="pageUrl(doc)">{{ doc.title }}</a>
                    </p>
                    <p
                      v-for="artifact in project.data.artifacts.filter(row => row.taskId === item.id)"
                      :key="artifact.id"
                    >
                      {{ t('artifacts') }}: {{ artifact.name }}
                    </p>
                    <button
                      v-if="project.canEdit"
                      :disabled="busy"
                      @click="task = JSON.parse(JSON.stringify(item))"
                    >
                      {{ t('edit') }}
                    </button>
                    <label v-if="project.canEdit">{{ t('status') }}<select
                      :value="item.status"
                      :disabled="busy"
                      @change="command('transition', { id: item.id, status: $event.target.value })"
                    ><option
                      v-for="next in statuses"
                      :key="next"
                      :value="next"
                    >{{ t(next) }}</option></select></label>
                  </article>
                </section>
              </div>
            </section>
            <credits-panel
              v-if="tab === 'credits'"
              :project="project"
              :t="t"
              :busy="busy"
              :command="command"
              :run="run"
            />
            <section v-if="tab === 'history'">
              <h3>{{ t('integrationEvents') }}</h3>
              <p
                v-if="!project.data.integrationEvents.length"
                class="studio-muted"
              >
                {{ t('noItems') }}
              </p>
              <p
                v-for="event in project.data.integrationEvents"
                :key="event.id"
              >
                {{ event.source }} · {{ event.type }} · {{ event.status || t('processed') }} · {{ event.at }}
              </p>
              <h3>{{ t('history') }}</h3>
              <p
                v-if="!project.data.history.length"
                class="studio-muted"
              >
                {{ t('noItems') }}
              </p>
              <p
                v-for="(event, index) in project.data.history.slice().reverse()"
                :key="index"
              >
                {{ event.at }} · {{ event.action }} · {{ event.actor }} {{ event.reason }}
              </p>
            </section>
          </section>
        </div>
      </section>
    </v-main>
    <nav-footer />
    <notify />
    <search-results />
  </v-app>
</template>

<script>
/* global siteConfig */
import gql from 'graphql-tag'
import messages from './messages'
import SearchPicker from './search-picker.vue'
import RecordForm from './record-form.vue'
import DeliveryPanel from './delivery-panel.vue'
import CreditsPanel from './credits-panel.vue'

const fields = 'id revision state canManage canEdit'
export default {
  components: { SearchPicker, RecordForm, DeliveryPanel, CreditsPanel },
  data () {
    const initialLocale = (document.documentElement.lang || (typeof siteConfig !== 'undefined' && siteConfig.lang) || 'en').toLowerCase()
    return {
      projects: [],
      selected: '',
      initialProjectId: '',
      busy: false,
      error: '',
      tab: 'overview',
      view: 'board',
      team: { name: '' },
      taskFilter: '',
      assigneeFilter: '',
      sections: ['overview', 'team', 'documents', 'work', 'planning', 'production', 'validation', 'releases', 'credits', 'history'],
      statuses: ['backlog', 'ready', 'active', 'review', 'done'],
      accessRoles: ['reader', 'editor', 'manager', 'remove'],
      newProject: { name: '', kind: 'digital' },
      person: { name: '', teamId: '', userId: null, roleText: '', order: 0 },
      task: { title: '', type: 'task', priority: 'normal', criteria: '', discipline: '', assigneeId: '', dependencies: [], documentIds: [], dueDate: '', blockedReason: '', milestoneId: '', cycleId: '' },
      access: { userId: null, role: 'reader' },
      document: { pageId: null },
      integration: { groupId: null, role: 'reader' },
      templates: [],
      newDocument: { template: 'gdd', title: '', path: '', locale: initialLocale }
    }
  },
  computed: {
    canCreateProject () {
      const perms = (this.$store && this.$store.get) ? (this.$store.get('user/permissions') || []) : []
      return Array.isArray(perms) && perms.includes('manage:system')
    },
    projectFields () {
      return [
        { key: 'name', required: true },
        { key: 'kind', required: true, options: ['digital', 'analogue', 'hybrid'].map(id => ({ id, name: this.t(id) })) }
      ]
    },
    taskFields () {
      const options = (key, collection, multiple = false) => ({
        key,
        multiple,
        options: this.project.data[collection]
          .filter(row => key !== 'dependencies' || row.id !== this.task.id)
          .map(row => ({ id: row.id, name: row.name || row.title }))
      })
      return [
        { key: 'title', label: 'task', required: true },
        { key: 'type', required: true, options: ['task', 'bug', 'feature'].map(id => ({ id, name: this.t(id) })) },
        { key: 'criteria', type: 'textarea' },
        { key: 'discipline' },
        options('assigneeId', 'participants'),
        { key: 'priority', required: true, options: ['low', 'normal', 'high', 'urgent'].map(id => ({ id, name: this.t(id) })) },
        { key: 'dueDate', type: 'date' },
        { key: 'blockedReason', type: 'textarea' },
        options('dependencies', 'tasks', true),
        options('milestoneId', 'milestones'),
        options('cycleId', 'cycles'),
        options('documentIds', 'documents', true)
      ]
    },
    project () {
      return this.projects.find(item => item.id === this.selected)
    },
    activePeople () {
      return this.project ? this.project.data.participants.filter(person => !person.endedAt) : []
    },
    openTasks () {
      return this.project ? this.project.data.tasks.filter(item => item.status !== 'done') : []
    },
    doneTasksCount () {
      return this.project ? this.project.data.tasks.filter(item => item.status === 'done').length : 0
    },
    blockedTasks () {
      if (!this.project) return []
      const doneIds = new Set(this.project.data.tasks.filter(row => row.status === 'done').map(row => row.id))
      return this.project.data.tasks.filter(item => item.status !== 'done' && (item.blockedReason || (item.dependencies || []).some(dep => !doneIds.has(dep))))
    },
    currentMilestone () {
      if (!this.project || !this.project.data.milestones.length) return null
      const dated = this.project.data.milestones.filter(m => m.dueDate).sort((a, b) => a.dueDate.localeCompare(b.dueDate))
      const today = new Date().toISOString().slice(0, 10)
      return dated.find(m => m.dueDate.slice(0, 10) >= today) || dated[dated.length - 1] || this.project.data.milestones[this.project.data.milestones.length - 1]
    },
    currentCycle () {
      if (!this.project || !this.project.data.cycles.length) return null
      const today = new Date().toISOString().slice(0, 10)
      return this.project.data.cycles.find(c => (!c.startDate || c.startDate.slice(0, 10) <= today) && (!c.endDate || c.endDate.slice(0, 10) >= today)) ||
        this.project.data.cycles[this.project.data.cycles.length - 1]
    },
    pendingArtifactVersions () {
      if (!this.project) return []
      return this.project.data.artifactVersions.filter(ver => {
        const reviews = this.project.data.artifactReviews.filter(r => r.versionId === ver.id)
        return !reviews.length || reviews[reviews.length - 1].decision !== 'approved'
      })
    },
    outdatedDocuments () {
      return this.project ? this.project.data.documents.filter(doc => doc.outdated) : []
    },
    recentReleases () {
      return this.project ? this.project.data.releases.slice(-5).reverse() : []
    }
  },
  created () {
    const lang = (document.documentElement.lang || (typeof siteConfig !== 'undefined' && siteConfig.lang) || 'en').toLowerCase()
    if (this.$store) {
      this.$store.commit('page/SET_MODE', 'studio')
      this.$store.commit('page/SET_LOCALE', lang)
    }
    const params = new URLSearchParams(window.location.search)
    const tabParam = params.get('tab')
    if (tabParam && this.sections.includes(tabParam)) {
      this.tab = tabParam
    }
    this.initialProjectId = params.get('project') || ''
  },
  mounted () {
    this.load()
  },
  methods: {
    t (key) {
      return messages[(document.documentElement.lang || '').startsWith('pt') ? 'pt' : 'en'][key] || key
    },
    syncUrl () {
      if (!window.history || !window.history.replaceState) return
      const params = new URLSearchParams(window.location.search)
      if (this.selected) params.set('project', this.selected)
      if (this.tab) params.set('tab', this.tab)
      window.history.replaceState(null, '', `${window.location.pathname}?${params.toString()}`)
    },
    setTab (section) {
      this.tab = section
      this.syncUrl()
    },
    async run (operation) {
      this.busy = true
      this.error = ''
      try {
        await operation()
        return true
      } catch (error) {
        this.error = error.message
        if (/revision|conflict|changed|stale/i.test(error.message)) {
          try {
            const { data } = await this.$apollo.query({ query: gql(`query { studioProjects { ${fields} } }`), fetchPolicy: 'network-only' })
            this.projects = data.studioProjects.map(this.decode)
            this.error = this.t('conflict')
          } catch (refreshError) {
            this.error += ' ' + refreshError.message
          }
        }
        return false
      } finally {
        this.busy = false
      }
    },
    decode (row) {
      return {
        ...row,
        data: {
          teams: [],
          milestones: [],
          cycles: [],
          artifacts: [],
          artifactVersions: [],
          artifactReviews: [],
          validations: [],
          integrationAccess: [],
          integrationEvents: [],
          ...JSON.parse(row.state)
        }
      }
    },
    async load () {
      await this.run(async () => {
        const { data } = await this.$apollo.query({ query: gql(`query { studioProjects { ${fields} } }`), fetchPolicy: 'network-only' })
        this.projects = data.studioProjects.map(this.decode)
        if (!this.project && this.projects.length) {
          const requested = this.initialProjectId && this.projects.find(item => item.id === this.initialProjectId)
          this.select(requested ? requested.id : this.projects[0].id)
          this.initialProjectId = ''
        }
        const result = await this.$apollo.query({
          query: gql('query($locale:String!){studioTemplates(locale:$locale){key title version}}'),
          variables: { locale: this.newDocument.locale }
        })
        this.templates = result.data.studioTemplates
      })
    },
    select (id) {
      this.selected = id
      this.person = { name: '', teamId: '', userId: null, roleText: '', order: 0 }
      this.resetTask()
      this.team = { name: '' }
      this.taskFilter = ''
      this.assigneeFilter = ''
      this.access = { userId: null, role: 'reader' }
      this.document = { pageId: null }
      this.integration = { groupId: null, role: 'reader' }
      this.syncUrl()
    },
    async createProject () {
      await this.run(async () => {
        const { data } = await this.$apollo.mutate({
          mutation: gql(`mutation ($name: String!, $kind: String!) { studioCreateProject(name: $name, kind: $kind) { ${fields} } }`),
          variables: this.newProject
        })
        this.projects.push(this.decode(data.studioCreateProject))
        this.select(data.studioCreateProject.id)
        this.newProject.name = ''
      })
    },
    async command (command, input) {
      return this.run(async () => {
        const { data } = await this.$apollo.mutate({
          mutation: gql(`mutation ($projectId: String!, $revision: Int!, $command: String!, $input: String!) { studioCommand(projectId: $projectId, revision: $revision, command: $command, input: $input) { ${fields} } }`),
          variables: { projectId: this.selected, revision: this.project.revision, command, input: JSON.stringify(input) }
        })
        this.$set(this.projects, this.projects.findIndex(row => row.id === data.studioCommand.id), this.decode(data.studioCommand))
      })
    },
    editPerson (person) {
      this.person = { ...person, roleText: person.roles.join(', ') }
    },
    async savePerson () {
      if (await this.command('participant', { ...this.person, team: '', roles: this.person.roleText.split(',').map(role => role.trim()).filter(Boolean) })) {
        this.person = { name: '', teamId: '', userId: null, roleText: '', order: 0 }
      }
    },
    resetTask () {
      this.task = { title: '', type: 'task', priority: 'normal', criteria: '', discipline: '', assigneeId: '', dependencies: [], documentIds: [], dueDate: '', blockedReason: '', milestoneId: '', cycleId: '' }
    },
    async saveTask (input) {
      return this.command('task', input)
    },
    async saveProject (input) {
      return this.command('project', { name: input.name, kind: input.kind, archived: !!input.archived })
    },
    async saveTeam () {
      if (await this.command('team', this.team)) this.team = { name: '' }
    },
    tasksFor (status) {
      return this.project.data.tasks.filter(item =>
        item.status === status &&
        (!this.assigneeFilter || item.assigneeId === this.assigneeFilter) &&
        (item.title + ' ' + item.criteria + ' ' + item.discipline).toLowerCase().includes(this.taskFilter.toLowerCase())
      )
    },
    personName (id) {
      const person = this.project.data.participants.find(person => person.id === id)
      return person ? person.name : this.t('unassigned')
    },
    artifactVersionLabel (ver) {
      const artifact = this.project.data.artifacts.find(row => row.id === ver.artifactId)
      return artifact ? artifact.name + ' · ' + ver.version : ver.version
    },
    pageUrl (page) {
      return '/' + encodeURIComponent(page.locale) + '/' + page.path.split('/').map(encodeURIComponent).join('/')
    }
  }
}
</script>

<style>
.studio {
  max-width: 1520px;
  margin: 0 auto;
  padding: 28px 32px;
  font-family: "Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}
.studio-heading,
.studio-tabs {
  display: flex;
  gap: 10px;
  align-items: center;
  flex-wrap: wrap;
  margin-bottom: 20px;
}
.studio-heading {
  justify-content: space-between;
  padding-bottom: 16px;
  border-bottom: 1px solid #d4d4d8;
}
.theme--dark .studio-heading {
  border-bottom-color: #26262a;
}
.studio h1 {
  font-size: 1.5rem;
  font-weight: 600;
  letter-spacing: -0.02em;
  margin: 0;
}
.theme--dark .studio h1,
.theme--dark .studio h2 {
  color: #f4f4f5;
}
.theme--dark .studio h3,
.theme--dark .studio h4 {
  color: #e4e4e7;
}
.studio h2,
.studio h3 {
  margin: 12px 0;
  font-weight: 600;
  letter-spacing: -0.01em;
}
.studio h4 {
  margin: 0 0 8px 0;
  font-weight: 600;
}
.studio-layout > * {
  min-width: 0;
}
.studio fieldset {
  min-inline-size: 0;
}
.studio label,
.studio h2,
.studio h3,
.studio h4 {
  overflow-wrap: anywhere;
}
.studio-layout {
  display: grid;
  grid-template-columns: 260px minmax(0, 1fr);
  gap: 28px;
}
.studio label {
  display: block;
  margin-bottom: 12px;
  font-size: 0.88rem;
  font-weight: 500;
}
.studio input,
.studio select,
.studio textarea {
  box-sizing: border-box;
  display: block;
  width: 100%;
  margin-top: 4px;
  padding: 9px 11px;
  border: 1px solid #d4d4d8;
  border-radius: 5px;
  color: inherit;
  background: #ffffff;
  font-size: 0.92rem;
}
.theme--dark .studio input,
.theme--dark .studio select,
.theme--dark .studio textarea {
  border-color: #2e2e34;
  background: #111114;
  color: #e4e4e7;
}
.studio select option {
  color: #18181b;
  background: #ffffff;
}
.theme--dark .studio select option {
  color: #f4f4f5;
  background: #18181c;
}
.studio button {
  padding: 8px 14px;
  background: #27272a;
  color: #f4f4f5;
  border: 1px solid #3f3f46;
  border-radius: 5px;
  margin: 0 8px 8px 0;
  font-size: 0.88rem;
  font-weight: 500;
  transition: background-color 0.15s ease, border-color 0.15s ease, color 0.15s ease;
}
.studio button:hover:not(:disabled) {
  background: #323238;
  border-color: #52525b;
}
.studio button[aria-pressed="true"] {
  background: #f4f4f5;
  color: #111114;
  border-color: #f4f4f5;
  font-weight: 600;
}
.theme--light .studio button[aria-pressed="true"] {
  background: #18181b;
  color: #fafafa;
  border-color: #18181b;
}
.studio button:disabled {
  opacity: 0.5;
  cursor: wait;
}
.studio :focus-visible {
  outline: 2px solid #a1a1aa;
  outline-offset: 2px;
}
.studio form,
.studio-card,
.studio-settings {
  padding: 16px;
  border: 1px solid #e4e4e7;
  border-radius: 6px;
  background: #fafafa;
  margin-bottom: 16px;
}
.theme--dark .studio form,
.theme--dark .studio-card,
.theme--dark .studio-settings {
  background: #18181c;
  border-color: #26262a;
}
.studio-settings summary {
  cursor: pointer;
  font-weight: 500;
  padding: 2px 0;
}
.studio-metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(210px, 1fr));
  gap: 14px;
  margin-bottom: 8px;
}
.studio-overview-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 16px;
}
.studio-filters {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 12px;
  margin-bottom: 12px;
}
.studio-board {
  display: grid;
  grid-template-columns: repeat(5, minmax(210px, 1fr));
  overflow-x: auto;
  gap: 14px;
}
.studio fieldset {
  border: 1px solid #d4d4d8;
  border-radius: 5px;
  padding: 12px;
  margin-bottom: 12px;
}
.theme--dark .studio fieldset {
  border-color: #2e2e34;
}
.studio .studio-check {
  display: flex;
  align-items: center;
  gap: 10px;
}
.studio-check input {
  width: auto;
  margin-top: 0;
}
.studio-error {
  background: #fef2f2;
  color: #991b1b;
  border: 1px solid #fecaca;
  border-radius: 6px;
  padding: 12px 16px;
  margin-bottom: 16px;
}
.theme--dark .studio-error {
  background: #2a1517;
  color: #fca5a5;
  border-color: #5c2429;
}
.studio-muted {
  color: #71717a;
  font-size: 0.88rem;
}
.theme--dark .studio-muted {
  color: #a1a1aa;
}
.studio-badge {
  display: inline-block;
  font-size: 0.75rem;
  font-weight: 500;
  padding: 2px 8px;
  margin-left: 8px;
  border-radius: 4px;
  border: 1px solid #d4d4d8;
  vertical-align: middle;
}
.theme--dark .studio-badge {
  background: #242429;
  border-color: #323238;
  color: #d4d4d8;
}
.studio a {
  color: inherit;
  text-decoration: underline;
  text-decoration-color: #71717a;
  text-underline-offset: 3px;
}
.studio a:hover {
  text-decoration-color: currentColor;
}
.studio .checksum {
  overflow-wrap: anywhere;
  font-size: 0.84rem;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
.studio .project-button {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  width: 100%;
  text-align: left;
}
.studio .project-meta {
  font-size: 0.76rem;
  opacity: 0.75;
}
@media (max-width: 860px) {
  .studio-layout {
    grid-template-columns: 1fr;
  }
  .studio {
    padding: 16px;
  }
}
</style>
