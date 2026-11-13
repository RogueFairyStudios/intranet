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
        <h4>{{ item.name || versionName(item) }}</h4>
        <p v-if="item.publishedAt">
          {{ t('published') }} · {{ item.publishedAt }}
        </p>
        <p v-if="item.criteria || item.goal || item.notes || item.comment">
          {{ item.criteria || item.goal || item.notes || item.comment }}
        </p>
        <p v-if="item.dueDate || item.startDate || item.endDate || item.status || item.result || item.decision">
          {{ (item.dueDate || item.startDate || '').slice(0, 10) }} {{ (item.endDate || '').slice(0, 10) }} {{ (item.status || item.result || item.decision) ? t(item.status || item.result || item.decision) : '' }}
        </p>
        <p
          v-if="item.uri"
          class="checksum"
        >
          <a
            v-if="/^https?:/.test(item.uri)"
            :href="item.uri"
            target="_blank"
            rel="noopener noreferrer"
          >{{ item.uri }}</a>
          <span v-else>{{ item.uri }}</span>
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

export default {
  components: { RecordForm },
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
        options: this.state[collection].map(item => ({ id: item.id, name: item.name || item.title || this.versionName(item) }))
      })
      const values = (key, list) => ({ key, required: true, options: list.map(id => ({ id, name: this.t(id) })) })
      if (this.section === 'planning') {
        return [
          { command: 'milestone', collection: 'milestones', label: 'milestones', editable: true, fields: [text('name', true), text('criteria', false, 'textarea'), text('dueDate', false, 'date')] },
          { command: 'cycle', collection: 'cycles', label: 'cycles', editable: true, fields: [text('name', true), text('startDate', true, 'date'), text('endDate', true, 'date'), text('goal', false, 'textarea'), { key: 'capacity', type: 'number', min: 0 }] }
        ]
      }
      if (this.section === 'production') {
        return [
          { command: 'artifact', collection: 'artifacts', label: 'artifacts', editable: true, fields: [text('name', true), text('kind', true), options('taskId', 'tasks'), options('documentIds', 'documents', true)] },
          { command: 'artifactVersion', collection: 'artifactVersions', label: 'versions', fields: [{ ...options('artifactId', 'artifacts'), required: true }, text('version', true), text('uri', true), text('previewUri'), text('checksum', true)] },
          { command: 'reviewArtifact', collection: 'artifactReviews', label: 'reviews', fields: [{ ...options('versionId', 'artifactVersions'), required: true }, values('decision', ['approved', 'changes']), text('comment', true, 'textarea')] }
        ]
      }
      if (this.section === 'validation') {
        return [
          { command: 'validation', collection: 'validations', label: 'validation', fields: [text('name', true), options('releaseId', 'releases'), options('artifactVersionId', 'artifactVersions'), values('kind', ['test', 'playtest']), values('result', ['pending', 'passed', 'failed']), text('notes', false, 'textarea'), options('taskId', 'tasks')] }
        ]
      }
      return [{ command: 'release', collection: 'releases', label: 'releases', editable: true, fields: [text('name', true), text('criteria', false, 'textarea'), options('artifactVersionIds', 'artifactVersions', true), options('validationIds', 'validations', true)] }]
    }
  },
  methods: {
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
