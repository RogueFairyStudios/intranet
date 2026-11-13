<template>
  <section>
    <p v-if="!project.data.credits.length">
      {{ t('noItems') }}
    </p>
    <form
      v-if="project.canEdit"
      @submit.prevent="saveRelease"
    >
      <label>{{ t('newRelease') }}<input
        v-model="release.name"
        required
      ></label><button :disabled="busy">
        {{ t('create') }}
      </button>
    </form>
    <form
      v-if="project.canEdit"
      @submit.prevent="command('generateCredits', creditSelection)"
    >
      <label>{{ t('release') }}<select
        v-model="creditSelection.releaseId"
        required
      ><option
        v-for="item in project.data.releases"
        :key="item.id"
        :value="item.id"
      >{{ item.name }}</option></select></label>
      <fieldset>
        <legend>{{ t('contributors') }}</legend><label
          v-for="member in project.data.participants"
          :key="member.id"
          class="studio-check"
        ><input
          v-model="creditSelection.participantIds"
          type="checkbox"
          :value="member.id"
        >{{ member.name }} · {{ member.roles.join(', ') }}</label>
      </fieldset>
      <button :disabled="busy">
        {{ t('generate') }}
      </button>
    </form>
    <article
      v-for="snapshot in project.data.credits"
      :key="snapshot.id"
      class="studio-card"
    >
      <h3>{{ snapshot.manifest.title }} · {{ t(snapshot.status) }}</h3>
      <p
        v-for="(entry, index) in snapshot.manifest.entries"
        :key="index"
      >
        {{ entry.section }} / {{ entry.role }} — {{ entry.name }}
      </p>
      <p class="checksum">
        SHA-256 {{ snapshot.checksum }}
      </p>
      <button
        v-if="snapshot.status === 'draft' && project.canEdit"
        :disabled="busy"
        @click="editSnapshot(snapshot)"
      >
        {{ t('edit') }}
      </button>
      <button
        v-if="snapshot.status === 'draft' && project.canManage"
        :disabled="busy"
        @click="command('approveCredits', { id: snapshot.id })"
      >
        {{ t('approve') }}
      </button>
      <template v-if="snapshot.status === 'approved'">
        <button
          v-if="project.canEdit"
          :disabled="busy"
          @click="command('reviseCredits', { id: snapshot.id })"
        >
          {{ t('reviseCredits') }}
        </button>
        <button
          :disabled="busy"
          @click="download(snapshot, 'json')"
        >
          JSON
        </button><button
          :disabled="busy"
          @click="download(snapshot, 'markdown')"
        >
          Markdown
        </button>
      </template>
    </article>
    <form
      v-if="creditEdit.id && project.canEdit"
      @submit.prevent="saveCredits"
    >
      <h3>{{ t('editCredits') }}</h3>
      <fieldset
        v-for="(entry, index) in creditEdit.entries"
        :key="index"
      >
        <legend>{{ index + 1 }}</legend><label>{{ t('publicName') }}<input
          v-model="entry.name"
          required
        ></label><label>{{ t('role') }}<input
          v-model="entry.role"
          required
        ></label><label>{{ t('section') }}<input
          v-model="entry.section"
          required
        ></label>
        <button
          type="button"
          :disabled="index === 0"
          @click="moveCredit(index, -1)"
        >
          {{ t('moveUp') }}
        </button>
        <button
          type="button"
          :disabled="index === creditEdit.entries.length - 1"
          @click="moveCredit(index, 1)"
        >
          {{ t('moveDown') }}
        </button>
      </fieldset>
      <label>{{ t('reason') }}<input
        v-model="creditEdit.reason"
        required
      ></label><button :disabled="busy">
        {{ t('save') }}
      </button>
    </form>
    <form
      v-if="project.canEdit"
      @submit.prevent="saveBuild"
    >
      <h3>{{ t('registerBuild') }}</h3>
      <label>{{ t('release') }}<select
        v-model="build.releaseId"
        required
      ><option
        v-for="item in project.data.releases"
        :key="item.id"
        :value="item.id"
      >{{ item.name }}</option></select></label>
      <label>{{ t('approvedCredits') }}<select
        v-model="build.snapshotId"
        required
      ><option
        v-for="snapshot in approvedCredits"
        :key="snapshot.id"
        :value="snapshot.id"
      >{{ snapshot.manifest.title }} · {{ snapshot.checksum.slice(0, 12) }}</option></select></label>
      <label>{{ t('buildName') }}<input
        v-model="build.name"
        required
      ></label>
      <file-upload
        :project-id="project.id"
        :busy="busy"
        :t="t"
        @uploaded="applyBuildUpload"
      />
      <label>{{ t('uri') }}<input
        v-model="build.uri"
        maxlength="4000"
      ></label>
      <label>{{ t('artifactChecksum') }}<input
        v-model="build.checksum"
        maxlength="64"
      ></label>
      <button :disabled="busy">
        {{ t('save') }}
      </button>
    </form>
    <article
      v-for="item in project.data.builds"
      :key="item.id"
      class="studio-card"
    >
      <h4>{{ item.name }}</h4>
      <p class="checksum">
        {{ t('creditsChecksum') }}: {{ item.checksum }}
      </p>
      <p
        v-if="item.artifactChecksum"
        class="checksum"
      >
        {{ t('artifactChecksum') }}: {{ item.artifactChecksum }}
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
    </article>
  </section>
</template>
<script>
import gql from 'graphql-tag'
import FileUpload from './file-upload.vue'

export default {
  components: { FileUpload },
  props: {
    project: { type: Object, required: true },
    busy: Boolean,
    t: { type: Function, required: true },
    command: { type: Function, required: true },
    run: { type: Function, required: true }
  },
  data () {
    return {
      release: { name: '' },
      creditSelection: { releaseId: '', participantIds: [] },
      creditEdit: { id: '', entries: [], reason: '' },
      build: { name: '', releaseId: '', snapshotId: '', uri: '', checksum: '' }
    }
  },
  computed: {
    approvedCredits () {
      return this.project.data.credits.filter(snapshot => snapshot.status === 'approved' && snapshot.releaseId === this.build.releaseId)
    }
  },
  watch: {
    'build.releaseId' () {
      this.build.snapshotId = ''
    }
  },
  methods: {
    applyBuildUpload ({ uri, checksum }) {
      this.build.uri = uri
      this.build.checksum = checksum
    },
    async saveRelease () {
      if (await this.command('release', this.release)) this.release.name = ''
    },
    async saveBuild () {
      const payload = {
        name: this.build.name,
        releaseId: this.build.releaseId,
        snapshotId: this.build.snapshotId
      }
      if (this.build.uri) payload.uri = this.build.uri
      if (this.build.checksum) payload.checksum = this.build.checksum
      if (await this.command('build', payload)) {
        this.build.name = ''
        this.build.uri = ''
        this.build.checksum = ''
      }
    },
    moveCredit (index, direction) {
      const entries = this.creditEdit.entries
      const entry = entries.splice(index, 1)[0]
      entries.splice(index + direction, 0, entry)
    },
    editSnapshot (snapshot) {
      this.creditEdit = { id: snapshot.id, entries: JSON.parse(JSON.stringify(snapshot.manifest.entries)), reason: '' }
    },
    async saveCredits () {
      if (await this.command('editCredits', this.creditEdit)) this.creditEdit.id = ''
    },
    async download (snapshot, format) {
      await this.run(async () => {
        const { data } = await this.$apollo.query({
          query: gql`
            query ($projectId: String!, $snapshotId: String!, $format: String!) {
              studioCredits(projectId: $projectId, snapshotId: $snapshotId, format: $format)
            }
          `,
          variables: { projectId: this.project.id, snapshotId: snapshot.id, format },
          fetchPolicy: 'network-only'
        })
        const url = URL.createObjectURL(new Blob([data.studioCredits], { type: format === 'json' ? 'application/json;charset=utf-8' : 'text/markdown;charset=utf-8' }))
        const link = document.createElement('a')
        link.href = url
        link.download = 'credits-' + snapshot.id + (format === 'json' ? '.json' : '.md')
        link.click()
        setTimeout(() => URL.revokeObjectURL(url), 1000)
      })
    }
  }
}
</script>
