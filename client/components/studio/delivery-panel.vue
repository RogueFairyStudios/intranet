<template>
  <section>
    <div
      v-for="group in groups"
      :key="group.command"
    >
      <h3>{{ t(group.label) }}</h3>
      <button
        v-if="project.canEdit && (group.command !== 'reviewArtifact' || project.canManage)"
        :disabled="busy"
        @click="edit(group)"
      >
        {{ t('create') }}
      </button>
      <record-form
        v-if="editing === group.command && project.canEdit"
        :title="t(group.label)"
        :fields="group.fields"
        :value="draft"
        :t="t"
        :busy="busy"
        :project-id="project.id"
        :enable-upload="group.command === 'artifactVersion'"
        :submit="save"
        @cancel="editing = ''"
      />
      <p v-if="!state[group.collection].length">
        {{ t('noItems') }}
      </p>
      <article
        v-for="item in state[group.collection]"
        :key="item.id"
        class="studio-card"
      >
        <h4>
          {{ item.name || versionName(item) }}
          <span
            v-if="item.phase"
            class="studio-badge"
          >{{ t(item.phase) }}</span>
          <span
            v-if="item.track"
            class="studio-badge"
          >{{ t(item.track) }}</span>
          <span
            v-if="item.stage"
            class="studio-badge"
          >{{ t(item.stage) }}</span>
          <span
            v-if="item.tool && item.tool !== 'generic'"
            class="studio-badge"
          >{{ t(item.tool) }}</span>
        </h4>
        <p v-if="item.publishedAt">
          {{ t('published') }} · {{ item.publishedAt }}
        </p>
        <p v-if="item.criteria || item.goal || item.notes || item.comment">
          {{ item.criteria || item.goal || item.notes || item.comment }}
        </p>
        <p v-if="item.dueDate || item.startDate || item.endDate || item.status || item.result || item.decision">
          {{ (item.dueDate || item.startDate || '').slice(0, 10) }} {{ (item.endDate || '').slice(0, 10) }} {{ (item.status || item.result || item.decision) ? t(item.status || item.result || item.decision) : '' }}
        </p>
        <div v-if="item.toolUri">
          <uri-link
            :uri="item.toolUri"
            :t="t"
          />
        </div>
        <div v-if="item.uri">
          <uri-link
            :uri="item.uri"
            :t="t"
          />
        </div>
        <p
          v-if="item.vcsBranch || item.vcsRevision"
          class="checksum"
        >
          {{ t('vcsRevision') }}: {{ item.vcsBranch ? item.vcsBranch + ' · ' : '' }}{{ item.vcsRevision || '' }}
        </p>
        <p
          v-if="item.checksum"
          class="checksum"
        >
          SHA-256 {{ item.checksum }}
        </p>
        <p v-if="item.taskId">
          {{ t('task') }}: {{ linkedName('tasks', item.taskId) }}
        </p>
        <p
          v-for="doc in state.documents.filter(row => (item.documentIds || []).includes(row.id))"
          :key="doc.id"
        >
          <a :href="'/' + encodeURIComponent(doc.locale) + '/' + doc.path.split('/').map(encodeURIComponent).join('/')">{{ doc.title }}</a>
        </p>
        <p
          v-for="versionId in item.artifactVersionIds || []"
          :key="versionId"
        >
          {{ t('versions') }}: {{ linkedName('artifactVersions', versionId) }}
        </p>
        <p
          v-for="validationId in item.validationIds || []"
          :key="validationId"
        >
          {{ t('validation') }}: {{ linkedName('validations', validationId) }}
        </p>
        <p
          v-for="target in (group.command === 'release' ? releaseTargetsFor(item) : [])"
          :key="target.id"
        >
          {{ t('distribution') }}: <strong>{{ target.name }}</strong>
          <span :class="['studio-badge', targetReady(target) ? 'studio-clean-badge' : 'studio-dirty-badge']">
            {{ targetReady(target) ? t('submissionReady') : t('submissionPending') }}
          </span>
        </p>
        <button
          v-if="project.canEdit && group.editable && !item.publishedAt"
          :disabled="busy"
          @click="edit(group, item)"
        >
          {{ t('edit') }}
        </button>
        <button
          v-if="group.command === 'release' && project.canManage && !item.publishedAt"
          :disabled="busy"
          @click="command('publishRelease', { id: item.id })"
        >
          {{ t('publish') }}
        </button>
      </article>
    </div>
  </section>
</template>
<script>
import RecordForm from './record-form.vue'
import UriLink from './uri-link.vue'

const phaseIds = ['pre_production', 'production', 'post_production', 'live']
const stageIds = ['concept', 'blockout', 'authoring', 'surface_loc', 'engine_integration', 'polish']
const trackIds = ['general', 'art_2d', 'art_3d', 'narrative', 'spirit', 'scene_level', 'audio_vfx', 'code_system', 'typography', 'tabletop_print']
const toolIds = ['generic', 'goethe', 'geppetto', 'godot', 'carnifex', 'animafaber', 'fontpit', 'lore', 'urc', 'uefn', 'unity', 'git', 'github']

export default {
  components: { RecordForm, UriLink },
  props: {
    project: { type: Object, required: true },
    section: { type: String, required: true },
    busy: Boolean,
    t: { type: Function, required: true },
    command: { type: Function, required: true }
  },
  data () {
    return { editing: '', draft: {} }
  },
  computed: {
    state () {
      return this.project.data
    },
    groups () {
      const text = (key, required = false, type) => ({ key, required, type })
      const options = (key, collection, multiple = false) => ({
        key,
        multiple,
        options: (this.state[collection] || []).map(item => ({ id: item.id, name: item.name || item.title || this.versionName(item) }))
      })
      const values = (key, list, required = true) => ({ key, required, options: list.map(id => ({ id, name: this.t(id) })) })
      if (this.section === 'planning') {
        return [
          { command: 'milestone', collection: 'milestones', label: 'milestones', editable: true, fields: [text('name', true), values('phase', phaseIds), text('criteria', false, 'textarea'), text('dueDate', false, 'date')] },
          { command: 'cycle', collection: 'cycles', label: 'cycles', editable: true, fields: [text('name', true), text('startDate', true, 'date'), text('endDate', true, 'date'), text('goal', false, 'textarea'), { key: 'capacity', type: 'number', min: 0 }] }
        ]
      }
      if (this.section === 'production') {
        return [
          { command: 'artifact', collection: 'artifacts', label: 'artifacts', editable: true, fields: [text('name', true), text('kind', true), values('track', trackIds), values('stage', stageIds), values('tool', toolIds), options('repoId', 'repositories'), { key: 'assetPath', maxlength: 1000 }, { key: 'toolUri', maxlength: 4000 }, options('taskId', 'tasks'), options('documentIds', 'documents', true)] },
          { command: 'artifactVersion', collection: 'artifactVersions', label: 'versions', fields: [{ ...options('artifactId', 'artifacts'), required: true }, text('version', true), { key: 'uri', required: true, maxlength: 4000 }, { key: 'previewUri', maxlength: 4000 }, { key: 'toolUri', maxlength: 4000 }, values('stage', stageIds, false), text('vcsBranch'), text('vcsRevision'), text('checksum', true)] },
          { command: 'reviewArtifact', collection: 'artifactReviews', label: 'reviews', fields: [{ ...options('versionId', 'artifactVersions'), required: true }, values('decision', ['approved', 'changes']), text('comment', true, 'textarea')] }
        ]
      }
      if (this.section === 'validation') {
        return [
          { command: 'validation', collection: 'validations', label: 'validation', fields: [text('name', true), options('releaseId', 'releases'), options('artifactVersionId', 'artifactVersions'), values('kind', ['test', 'playtest']), values('result', ['pending', 'passed', 'failed']), text('notes', false, 'textarea'), options('taskId', 'tasks')] }
        ]
      }
      return [{ command: 'release', collection: 'releases', label: 'releases', editable: true, fields: [text('name', true), text('criteria', false, 'textarea'), options('artifactVersionIds', 'artifactVersions', true), options('validationIds', 'validations', true), options('platformTargetIds', 'platformTargets', true)] }]
    }
  },
  methods: {
    releaseTargetsFor (item) {
      if (!item || !Array.isArray(this.state.platformTargets)) return []
      const explicit = new Set(item.platformTargetIds || [])
      return this.state.platformTargets.filter(target => !target.archived && (target.releaseId === item.id || explicit.has(target.id)))
    },
    targetReady (target) {
      const checksOk = (target.checklist || []).filter(c => c.required !== false).every(c => c.status === 'done' || (c.status === 'na' && c.waiverReason))
      const assetsOk = (target.assets || []).filter(a => a.required !== false).every(a => (a.status === 'done' && (a.artifactVersionId || a.uri)) || (a.status === 'na' && a.waiverReason))
      return checksOk && assetsOk
    },
    linkedName (collection, id) {
      const item = this.state[collection].find(row => row.id === id)
      return item ? item.name || item.title || this.versionName(item) : this.t('none')
    },
    versionName (item) {
      if (item.versionId) return this.versionName(this.state.artifactVersions.find(row => row.id === item.versionId) || {})
      const artifact = this.state.artifacts.find(row => row.id === item.artifactId)
      return artifact ? artifact.name + ' · ' + item.version : item.version || this.t('review')
    },
    edit (group, item) {
      this.editing = group.command
      this.draft = item ?
        JSON.parse(JSON.stringify(item)) :
        group.fields.reduce((draft, field) => {
          draft[field.key] = field.multiple ? [] : field.options && field.required ? (field.options[0] || {}).id || '' : ''
          return draft
        }, {})
    },
    async save (input) {
      return this.command(this.editing, input)
    }
  }
}
</script>
