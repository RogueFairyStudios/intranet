<template>
  <section class="studio-distribution">
    <p class="studio-muted">
      {{ t('distributionIntro') }}
    </p>

    <details
      v-if="project.canEdit"
      :open="!visibleTargets.length || !!targetForm.id"
      class="studio-settings"
    >
      <summary>{{ targetForm.id ? t('edit') + ' · ' + targetForm.name : t('newPlatformTarget') }}</summary>
      <form
        class="studio-inline-form"
        @submit.prevent="saveTarget"
      >
        <div class="studio-form-grid">
          <label>{{ t('storefront') }}<select
            v-model="targetForm.storefront"
            :disabled="!!targetForm.id"
            @change="onStorefrontChange"
          ><option
            v-for="sf in storefronts"
            :key="sf"
            :value="sf"
          >{{ t(sf) }}</option></select></label>
          <label>{{ t('name') }}<input
            v-model="targetForm.name"
            required
            maxlength="200"
          ></label>
          <label>{{ t('release') }}<select v-model="targetForm.releaseId"><option value="">{{ t('none') }}</option><option
            v-for="rel in state.releases"
            :key="rel.id"
            :value="rel.id"
          >{{ rel.name }}</option></select></label>
          <label>{{ t('submissionStatus') }}<select v-model="targetForm.submissionStatus"><option
            v-for="st in submissionStatuses"
            :key="st"
            :value="st"
          >{{ subStatusLabel(st) }}</option></select></label>
          <label>{{ t('appIdentifier') }}<input
            v-model="targetForm.appIdentifier"
            maxlength="200"
            placeholder="AppID 1234560 · net.roguefairy.game"
          ></label>
          <label>{{ t('targetDate') }}<input
            v-model="targetForm.targetDate"
            type="date"
          ></label>
        </div>
        <label>{{ t('storeUrl') }}<input
          v-model="targetForm.storeUrl"
          maxlength="2000"
          placeholder="https://store.steampowered.com/app/…"
        ></label>
        <label
          v-if="!targetForm.id && targetForm.storefront !== 'custom'"
          class="studio-check"
        ><input
          v-model="targetForm.applyPreset"
          type="checkbox"
        >{{ t('applyPreset') }}</label>
        <div>
          <button :disabled="busy">
            {{ t('save') }}
          </button>
          <button
            v-if="targetForm.id"
            type="button"
            :disabled="busy"
            @click="resetTargetForm"
          >
            {{ t('cancel') }}
          </button>
        </div>
      </form>
    </details>

    <p
      v-if="!visibleTargets.length"
      class="studio-muted"
    >
      {{ t('noItems') }}
    </p>

    <div
      v-if="visibleTargets.length"
      class="studio-repo-grid"
    >
      <article
        v-for="target in visibleTargets"
        :key="target.id"
        :class="['studio-card', 'studio-phase-card', activeTargetId === target.id ? 'is-active' : '']"
      >
        <div class="studio-asset-header">
          <h4>{{ target.name }}</h4>
          <span class="studio-badge">{{ t(target.storefront) }}</span>
          <span class="studio-badge">{{ subStatusLabel(target.submissionStatus) }}</span>
          <span :class="['studio-badge', readiness(target).ready ? 'studio-clean-badge' : 'studio-dirty-badge']">
            {{ readiness(target).ready ? t('submissionReady') : t('submissionPending') + ' (' + readiness(target).missingCount + ')' }}
          </span>
        </div>
        <p class="studio-muted">
          <span v-if="target.releaseId">{{ t('release') }}: <strong>{{ releaseName(target.releaseId) }}</strong> · </span>
          <span
            v-if="target.appIdentifier"
            class="checksum"
          >{{ target.appIdentifier }} · </span>
          <span v-if="target.targetDate">{{ t('dueDate') }}: {{ target.targetDate.slice(0, 10) }}</span>
        </p>
        <div v-if="target.storeUrl">
          <uri-link
            :uri="target.storeUrl"
            :t="t"
            compact
          />
        </div>
        <p class="studio-muted">
          {{ t('technical') }}: <strong>{{ categoryDone(target, 'technical') }}/{{ categoryTotal(target, 'technical') }}</strong> ·
          {{ t('bureaucratic') }}: <strong>{{ categoryDone(target, 'bureaucratic') }}/{{ categoryTotal(target, 'bureaucratic') }}</strong> ·
          {{ t('storefrontAssets') }}: <strong>{{ readiness(target).assetsDone }}/{{ readiness(target).assetsTotal }}</strong>
        </p>
        <div>
          <button
            type="button"
            :aria-pressed="activeTargetId === target.id"
            @click="activeTargetId = target.id"
          >
            {{ t('openSection') }}
          </button>
          <button
            v-if="project.canEdit"
            type="button"
            :disabled="busy"
            @click="editTarget(target)"
          >
            {{ t('edit') }}
          </button>
          <button
            v-if="project.canEdit"
            type="button"
            :disabled="busy"
            @click="toggleArchiveTarget(target)"
          >
            {{ t(target.archived ? 'restore' : 'archive') }}
          </button>
        </div>
      </article>
    </div>

    <div
      v-if="currentTarget"
      class="studio-platform-detail"
    >
      <div class="studio-heading">
        <h3>
          {{ currentTarget.name }}
          <small class="studio-badge">{{ t(currentTarget.storefront) }}</small>
        </h3>
      </div>

      <div class="studio-filters">
        <label>{{ t('search') }}<input
          v-model="itemFilter"
          type="search"
        ></label>
        <label>{{ t('status') }}<select v-model="statusFilter"><option value="">{{ t('all') }}</option><option
          v-for="st in itemStatuses"
          :key="st"
          :value="st"
        >{{ itemStatusLabel(st) }}</option></select></label>
      </div>

      <div
        v-for="cat in ['technical', 'bureaucratic']"
        :key="cat"
        class="studio-platform-block"
      >
        <h3>
          {{ t(cat === 'technical' ? 'technicalChecklist' : 'bureaucraticChecklist') }}
          <span class="studio-badge">{{ categoryDone(currentTarget, cat) }} / {{ categoryTotal(currentTarget, cat) }}</span>
        </h3>
        <details
          v-if="project.canEdit"
          class="studio-settings"
        >
          <summary>{{ t('newCheckItem') }} · {{ t(cat) }}</summary>
          <form
            class="studio-inline-form"
            @submit.prevent="addCheckItem(cat)"
          >
            <div class="studio-form-grid">
              <label>{{ t('title') }}<input
                v-model="newCheck[cat].title"
                required
                maxlength="300"
              ></label>
              <label>{{ t('assigneeId') }}<select v-model="newCheck[cat].assigneeId"><option value="">{{ t('none') }}</option><option
                v-for="p in state.participants"
                :key="p.id"
                :value="p.id"
              >{{ p.name }}</option></select></label>
              <label>{{ t('documents') }}<select v-model="newCheck[cat].documentId"><option value="">{{ t('none') }}</option><option
                v-for="d in state.documents"
                :key="d.id"
                :value="d.id"
              >{{ d.title }}</option></select></label>
              <label>{{ t('taskId') }}<select v-model="newCheck[cat].taskId"><option value="">{{ t('none') }}</option><option
                v-for="tk in state.tasks"
                :key="tk.id"
                :value="tk.id"
              >{{ tk.title }}</option></select></label>
            </div>
            <label>{{ t('specification') }}<textarea
              v-model="newCheck[cat].spec"
              maxlength="2000"
            /></label>
            <label class="studio-check"><input
              v-model="newCheck[cat].required"
              type="checkbox"
            >{{ t('requiredItem') }}</label>
            <button :disabled="busy">
              {{ t('create') }}
            </button>
          </form>
        </details>

        <p
          v-if="!filteredChecks(cat).length"
          class="studio-muted"
        >
          {{ t('noItems') }}
        </p>
        <article
          v-for="chk in filteredChecks(cat)"
          :key="chk.id"
          class="studio-card"
        >
          <div class="studio-asset-header">
            <strong>{{ chk.title }}</strong>
            <span class="studio-badge">{{ chk.required !== false ? t('requiredItem') : t('optionalItem') }}</span>
            <span :class="['studio-badge', statusBadgeClass(chk.status)]">{{ itemStatusLabel(chk.status) }}</span>
          </div>
          <p
            v-if="chk.spec"
            class="studio-muted"
          >
            {{ chk.spec }}
          </p>
          <p
            v-if="chk.waiverReason"
            class="studio-muted"
          >
            {{ t('waiverReason') }}: {{ chk.waiverReason }}
          </p>
          <p
            v-if="chk.notes"
            class="studio-muted"
          >
            {{ chk.notes }}
          </p>
          <p
            v-if="chk.assigneeId || chk.taskId || chk.documentId || chk.verifiedAt"
            class="studio-muted"
          >
            <span v-if="chk.assigneeId">{{ personName(chk.assigneeId) }} · </span>
            <span v-if="chk.taskId">{{ t('task') }}: {{ taskName(chk.taskId) }} · </span>
            <a
              v-if="chk.documentId && docById(chk.documentId)"
              :href="docUrl(docById(chk.documentId))"
            >{{ docById(chk.documentId).title }}</a>
            <span v-if="chk.verifiedAt"> · {{ t('verifiedByLabel') }} {{ chk.verifiedBy }} ({{ chk.verifiedAt.slice(0, 10) }})</span>
          </p>

          <div
            v-if="project.canEdit && editingCheckId !== chk.id"
            class="studio-inline-actions"
          >
            <button
              v-for="st in ['pending', 'in_progress', 'done', 'na']"
              :key="st"
              type="button"
              class="studio-btn-inline"
              :aria-pressed="chk.status === st"
              :disabled="busy"
              @click="onQuickCheckStatus(chk, st)"
            >
              {{ itemStatusLabel(st) }}
            </button>
            <button
              type="button"
              class="studio-btn-inline"
              :disabled="busy"
              @click="openCheckEditor(chk)"
            >
              {{ t('edit') }}
            </button>
          </div>

          <form
            v-if="project.canEdit && editingCheckId === chk.id"
            class="studio-inline-form"
            @submit.prevent="saveCheckEdit"
          >
            <div class="studio-form-grid">
              <label>{{ t('title') }}<input
                v-model="checkEdit.title"
                required
                maxlength="300"
              ></label>
              <label>{{ t('status') }}<select v-model="checkEdit.status"><option
                v-for="st in itemStatuses"
                :key="st"
                :value="st"
              >{{ itemStatusLabel(st) }}</option></select></label>
              <label>{{ t('assigneeId') }}<select v-model="checkEdit.assigneeId"><option value="">{{ t('none') }}</option><option
                v-for="p in state.participants"
                :key="p.id"
                :value="p.id"
              >{{ p.name }}</option></select></label>
              <label>{{ t('documents') }}<select v-model="checkEdit.documentId"><option value="">{{ t('none') }}</option><option
                v-for="d in state.documents"
                :key="d.id"
                :value="d.id"
              >{{ d.title }}</option></select></label>
            </div>
            <label v-if="checkEdit.status === 'na'">{{ t('waiverReason') }}<input
              v-model="checkEdit.waiverReason"
              :required="checkEdit.required !== false"
              maxlength="2000"
            ></label>
            <label>{{ t('specification') }}<textarea
              v-model="checkEdit.spec"
              maxlength="2000"
            /></label>
            <label>{{ t('notes') }}<input
              v-model="checkEdit.notes"
              maxlength="4000"
            ></label>
            <div>
              <button :disabled="busy">
                {{ t('save') }}
              </button>
              <button
                type="button"
                :disabled="busy"
                @click="editingCheckId = ''"
              >
                {{ t('cancel') }}
              </button>
            </div>
          </form>
        </article>
      </div>

      <div class="studio-platform-block">
        <h3>
          {{ t('storefrontAssets') }}
          <span class="studio-badge">{{ readiness(currentTarget).assetsDone }} / {{ readiness(currentTarget).assetsTotal }}</span>
        </h3>
        <details
          v-if="project.canEdit"
          class="studio-settings"
        >
          <summary>{{ t('newAssetSlot') }}</summary>
          <form
            class="studio-inline-form"
            @submit.prevent="addAssetSlot"
          >
            <div class="studio-form-grid">
              <label>{{ t('name') }}<input
                v-model="newSlot.name"
                required
                maxlength="200"
                placeholder="Capsule / Banner / Trailer…"
              ></label>
              <label>{{ t('dimensions') }}<input
                v-model="newSlot.dimensions"
                maxlength="120"
                placeholder="1920×1080 px"
              ></label>
              <label>{{ t('assetFormat') }}<input
                v-model="newSlot.format"
                maxlength="120"
                placeholder="PNG / JPG / MP4"
              ></label>
            </div>
            <label>{{ t('specification') }}<textarea
              v-model="newSlot.spec"
              maxlength="2000"
            /></label>
            <label class="studio-check"><input
              v-model="newSlot.required"
              type="checkbox"
            >{{ t('requiredItem') }}</label>
            <button :disabled="busy">
              {{ t('create') }}
            </button>
          </form>
        </details>

        <p
          v-if="!filteredAssets.length"
          class="studio-muted"
        >
          {{ t('noItems') }}
        </p>
        <article
          v-for="slot in filteredAssets"
          :key="slot.id"
          class="studio-card"
        >
          <div class="studio-asset-header">
            <strong>{{ slot.name }}</strong>
            <span
              v-if="slot.dimensions"
              class="studio-badge checksum"
            >{{ slot.dimensions }}</span>
            <span
              v-if="slot.format"
              class="studio-badge"
            >{{ slot.format }}</span>
            <span class="studio-badge">{{ slot.required !== false ? t('requiredItem') : t('optionalItem') }}</span>
            <span :class="['studio-badge', statusBadgeClass(slot.status)]">{{ itemStatusLabel(slot.status) }}</span>
          </div>
          <p
            v-if="slot.spec"
            class="studio-muted"
          >
            {{ slot.spec }}
          </p>
          <div v-if="slot.toolUri">
            <uri-link
              :uri="slot.toolUri"
              :t="t"
              compact
            />
          </div>
          <div v-if="slot.uri">
            <uri-link
              :uri="slot.uri"
              :t="t"
            />
          </div>
          <p
            v-if="slot.artifactVersionId"
            class="studio-muted"
          >
            {{ t('versions') }}: <strong>{{ versionLabel(slot.artifactVersionId) }}</strong>
          </p>
          <p
            v-if="slot.checksum"
            class="checksum"
          >
            SHA-256 {{ slot.checksum }}
          </p>
          <p
            v-if="slot.waiverReason"
            class="studio-muted"
          >
            {{ t('waiverReason') }}: {{ slot.waiverReason }}
          </p>
          <p
            v-if="slot.notes"
            class="studio-muted"
          >
            {{ slot.notes }}
          </p>

          <div v-if="project.canEdit && editingSlotId !== slot.id">
            <button
              type="button"
              class="studio-btn-inline"
              :disabled="busy"
              @click="openSlotEditor(slot)"
            >
              {{ t('edit') }}
            </button>
          </div>

          <form
            v-if="project.canEdit && editingSlotId === slot.id"
            class="studio-inline-form"
            @submit.prevent="saveSlotEdit"
          >
            <file-upload
              :project-id="project.id"
              :busy="busy"
              :t="t"
              @uploaded="applySlotUpload"
            />
            <div class="studio-form-grid">
              <label>{{ t('status') }}<select v-model="slotEdit.status"><option
                v-for="st in itemStatuses"
                :key="st"
                :value="st"
              >{{ itemStatusLabel(st) }}</option></select></label>
              <label>{{ t('artifactVersionId') }}<select
                v-model="slotEdit.artifactVersionId"
                @change="onSlotVersionSelect"
              ><option value="">{{ t('none') }}</option><option
                v-for="ver in state.artifactVersions"
                :key="ver.id"
                :value="ver.id"
              >{{ versionLabel(ver.id) }}</option></select></label>
              <label>{{ t('dimensions') }}<input
                v-model="slotEdit.dimensions"
                maxlength="120"
              ></label>
              <label>{{ t('assetFormat') }}<input
                v-model="slotEdit.format"
                maxlength="120"
              ></label>
              <label>{{ t('repoId') }}<select v-model="slotEdit.helperRepoId"><option value="">{{ t('none') }}</option><option
                v-for="r in state.repositories"
                :key="r.id"
                :value="r.id"
              >{{ r.name }} ({{ r.slug }})</option></select></label>
              <label>{{ t('sourceKraPath') }}<input
                v-model="slotEdit.helperKraPath"
                maxlength="500"
                placeholder="marketing/steam/header_capsule.kra"
              ></label>
            </div>
            <label>{{ t('toolUri') }}<input
              v-model="slotEdit.toolUri"
              maxlength="4000"
              placeholder="geppetto://open?repo=…&path=…"
            ></label>
            <label>{{ t('uri') }}<input
              v-model="slotEdit.uri"
              maxlength="4000"
              placeholder="https://… or uploaded private file URL"
            ></label>
            <label>{{ t('checksum') }}<input
              v-model="slotEdit.checksum"
              maxlength="64"
            ></label>
            <label v-if="slotEdit.status === 'na'">{{ t('waiverReason') }}<input
              v-model="slotEdit.waiverReason"
              :required="slotEdit.required !== false"
              maxlength="2000"
            ></label>
            <label>{{ t('notes') }}<input
              v-model="slotEdit.notes"
              maxlength="4000"
            ></label>
            <div>
              <button
                type="button"
                :disabled="busy"
                @click="composeSlotGeppettoUri"
              >
                {{ t('composeGeppettoUri') }}
              </button>
              <button :disabled="busy">
                {{ t('save') }}
              </button>
              <button
                type="button"
                :disabled="busy"
                @click="editingSlotId = ''"
              >
                {{ t('cancel') }}
              </button>
            </div>
          </form>
        </article>
      </div>
    </div>
  </section>
</template>
<script>
import FileUpload from './file-upload.vue'
import UriLink from './uri-link.vue'

const storefronts = [
  'steam',
  'epic',
  'apple_app_store',
  'google_play',
  'playstation',
  'xbox',
  'nintendo',
  'igdb',
  'itch_gog',
  'bgg_tabletop',
  'custom'
]

const submissionStatuses = [
  'preparing',
  'in_review',
  'changes_requested',
  'approved',
  'live'
]

const itemStatuses = ['pending', 'in_progress', 'done', 'na']

function emptyTarget (defaultName = '') {
  return {
    id: '',
    storefront: 'steam',
    name: defaultName,
    releaseId: '',
    storeUrl: '',
    appIdentifier: '',
    submissionStatus: 'preparing',
    targetDate: '',
    applyPreset: true
  }
}

export default {
  components: { FileUpload, UriLink },
  props: {
    project: { type: Object, required: true },
    busy: Boolean,
    t: { type: Function, required: true },
    command: { type: Function, required: true }
  },
  data () {
    return {
      storefronts,
      submissionStatuses,
      itemStatuses,
      activeTargetId: '',
      itemFilter: '',
      statusFilter: '',
      targetForm: emptyTarget(this.t('steam')),
      newCheck: {
        technical: { title: '', spec: '', required: true, assigneeId: '', documentId: '', taskId: '' },
        bureaucratic: { title: '', spec: '', required: true, assigneeId: '', documentId: '', taskId: '' }
      },
      newSlot: { name: '', dimensions: '', format: '', spec: '', required: true },
      editingCheckId: '',
      checkEdit: {},
      editingSlotId: '',
      slotEdit: {}
    }
  },
  computed: {
    state () {
      return this.project.data
    },
    visibleTargets () {
      return (this.state.platformTargets || []).filter(row => !row.archived)
    },
    currentTarget () {
      return this.visibleTargets.find(row => row.id === this.activeTargetId) || this.visibleTargets[0] || null
    },
    filteredAssets () {
      if (!this.currentTarget) return []
      const needle = this.itemFilter.trim().toLowerCase()
      return (this.currentTarget.assets || []).filter(slot => {
        if (this.statusFilter && slot.status !== this.statusFilter) return false
        if (!needle) return true
        return [slot.name, slot.dimensions, slot.format, slot.spec].filter(Boolean).join(' ').toLowerCase().includes(needle)
      })
    }
  },
  watch: {
    visibleTargets: {
      immediate: true,
      handler (list) {
        if (list.length && !list.some(item => item.id === this.activeTargetId)) {
          this.activeTargetId = list[0].id
        }
      }
    }
  },
  methods: {
    subStatusLabel (st) {
      return st === 'live' ? this.t('live_store') : this.t(st)
    },
    itemStatusLabel (st) {
      return this.t(st)
    },
    statusBadgeClass (st) {
      if (st === 'done') return 'studio-clean-badge'
      if (st === 'in_progress') return 'studio-lock-badge'
      if (st === 'pending') return 'studio-dirty-badge'
      return ''
    },
    onStorefrontChange () {
      if (!this.targetForm.id) {
        this.targetForm.name = this.t(this.targetForm.storefront)
      }
    },
    editTarget (target) {
      this.targetForm = {
        id: target.id,
        storefront: target.storefront,
        name: target.name,
        releaseId: target.releaseId || '',
        storeUrl: target.storeUrl || '',
        appIdentifier: target.appIdentifier || '',
        submissionStatus: target.submissionStatus || 'preparing',
        targetDate: target.targetDate ? target.targetDate.slice(0, 10) : '',
        applyPreset: false
      }
    },
    resetTargetForm () {
      this.targetForm = emptyTarget(this.t('steam'))
    },
    async saveTarget () {
      const locale = (document.documentElement.lang || 'pt').toLowerCase()
      const payload = {
        storefront: this.targetForm.storefront,
        name: this.targetForm.name,
        releaseId: this.targetForm.releaseId || null,
        storeUrl: this.targetForm.storeUrl || '',
        appIdentifier: this.targetForm.appIdentifier || '',
        submissionStatus: this.targetForm.submissionStatus,
        targetDate: this.targetForm.targetDate || null,
        applyPreset: this.targetForm.applyPreset,
        locale
      }
      if (this.targetForm.id) payload.id = this.targetForm.id
      const prevCount = (this.state.platformTargets || []).length
      if (await this.command('platformTarget', payload)) {
        if (!payload.id && (this.state.platformTargets || []).length > prevCount) {
          this.activeTargetId = this.state.platformTargets[this.state.platformTargets.length - 1].id
        }
        this.resetTargetForm()
      }
    },
    toggleArchiveTarget (target) {
      return this.command('platformTarget', {
        id: target.id,
        storefront: target.storefront,
        name: target.name,
        archived: !target.archived
      })
    },
    readiness (target) {
      const reqChecks = (target.checklist || []).filter(c => c.required !== false)
      const doneChecks = reqChecks.filter(c => c.status === 'done' || (c.status === 'na' && c.waiverReason)).length
      const reqAssets = (target.assets || []).filter(a => a.required !== false)
      const doneAssets = reqAssets.filter(a => (a.status === 'done' && (a.artifactVersionId || a.uri)) || (a.status === 'na' && a.waiverReason)).length
      const missingCount = (reqChecks.length - doneChecks) + (reqAssets.length - doneAssets)
      return {
        ready: missingCount === 0,
        checksDone: doneChecks,
        checksTotal: reqChecks.length,
        assetsDone: doneAssets,
        assetsTotal: reqAssets.length,
        missingCount
      }
    },
    categoryTotal (target, category) {
      return (target.checklist || []).filter(c => c.category === category && c.required !== false).length
    },
    categoryDone (target, category) {
      return (target.checklist || []).filter(c => c.category === category && c.required !== false && (c.status === 'done' || (c.status === 'na' && c.waiverReason))).length
    },
    filteredChecks (category) {
      if (!this.currentTarget) return []
      const needle = this.itemFilter.trim().toLowerCase()
      return (this.currentTarget.checklist || []).filter(chk => {
        if (chk.category !== category) return false
        if (this.statusFilter && chk.status !== this.statusFilter) return false
        if (!needle) return true
        return [chk.title, chk.spec, chk.notes].filter(Boolean).join(' ').toLowerCase().includes(needle)
      })
    },
    async addCheckItem (category) {
      const form = this.newCheck[category]
      const payload = {
        targetId: this.currentTarget.id,
        category,
        title: form.title,
        spec: form.spec,
        required: form.required,
        assigneeId: form.assigneeId || null,
        documentId: form.documentId || null,
        taskId: form.taskId || null
      }
      if (await this.command('platformCheckItem', payload)) {
        this.newCheck[category] = { title: '', spec: '', required: true, assigneeId: '', documentId: '', taskId: '' }
      }
    },
    onQuickCheckStatus (chk, nextStatus) {
      if (chk.status === nextStatus) return
      if (nextStatus === 'na' && chk.required !== false) {
        this.openCheckEditor(chk, 'na')
        return
      }
      return this.command('platformCheckItem', {
        targetId: this.currentTarget.id,
        id: chk.id,
        status: nextStatus
      })
    },
    openCheckEditor (chk, forcedStatus) {
      this.editingCheckId = chk.id
      this.checkEdit = {
        id: chk.id,
        category: chk.category,
        title: chk.title,
        spec: chk.spec || '',
        required: chk.required !== false,
        status: forcedStatus || chk.status,
        waiverReason: chk.waiverReason || '',
        assigneeId: chk.assigneeId || '',
        documentId: chk.documentId || '',
        notes: chk.notes || ''
      }
    },
    async saveCheckEdit () {
      const payload = {
        targetId: this.currentTarget.id,
        id: this.checkEdit.id,
        category: this.checkEdit.category,
        title: this.checkEdit.title,
        spec: this.checkEdit.spec,
        required: this.checkEdit.required,
        status: this.checkEdit.status,
        waiverReason: this.checkEdit.waiverReason,
        assigneeId: this.checkEdit.assigneeId || null,
        documentId: this.checkEdit.documentId || null,
        notes: this.checkEdit.notes
      }
      if (await this.command('platformCheckItem', payload)) {
        this.editingCheckId = ''
      }
    },
    async addAssetSlot () {
      const payload = {
        targetId: this.currentTarget.id,
        name: this.newSlot.name,
        dimensions: this.newSlot.dimensions,
        format: this.newSlot.format,
        spec: this.newSlot.spec,
        required: this.newSlot.required
      }
      if (await this.command('platformAssetSlot', payload)) {
        this.newSlot = { name: '', dimensions: '', format: '', spec: '', required: true }
      }
    },
    openSlotEditor (slot) {
      this.editingSlotId = slot.id
      this.slotEdit = {
        id: slot.id,
        name: slot.name,
        dimensions: slot.dimensions || '',
        format: slot.format || '',
        spec: slot.spec || '',
        required: slot.required !== false,
        status: slot.status,
        waiverReason: slot.waiverReason || '',
        artifactVersionId: slot.artifactVersionId || '',
        uri: slot.uri || '',
        checksum: slot.checksum || '',
        toolUri: slot.toolUri || '',
        notes: slot.notes || '',
        helperRepoId: (this.state.repositories[0] || {}).id || '',
        helperKraPath: ''
      }
    },
    applySlotUpload ({ uri, checksum }) {
      this.$set(this.slotEdit, 'uri', uri)
      this.$set(this.slotEdit, 'checksum', checksum)
      this.$set(this.slotEdit, 'status', 'done')
    },
    onSlotVersionSelect () {
      if (this.slotEdit.artifactVersionId && this.slotEdit.status === 'pending') {
        this.slotEdit.status = 'done'
      }
    },
    composeSlotGeppettoUri () {
      const repo = this.state.repositories.find(r => r.id === this.slotEdit.helperRepoId)
      const repoKey = repo ? (repo.workspacePath || repo.slug) : String(this.state.name || 'project').toLowerCase().replace(/[^a-z0-9._-]+/g, '-')
      const rel = (this.slotEdit.helperKraPath || '').trim().replace(/\\/g, '/').replace(/^\/+/, '')
      if (!rel) return
      const enc = encodeURIComponent
      this.slotEdit.toolUri = `geppetto://open?repo=${enc(repoKey)}&path=${enc(rel)}`
    },
    async saveSlotEdit () {
      const payload = {
        targetId: this.currentTarget.id,
        id: this.slotEdit.id,
        name: this.slotEdit.name,
        dimensions: this.slotEdit.dimensions,
        format: this.slotEdit.format,
        spec: this.slotEdit.spec,
        required: this.slotEdit.required,
        status: this.slotEdit.status,
        waiverReason: this.slotEdit.waiverReason,
        artifactVersionId: this.slotEdit.artifactVersionId || null,
        uri: this.slotEdit.uri || '',
        checksum: this.slotEdit.checksum || '',
        toolUri: this.slotEdit.toolUri || '',
        notes: this.slotEdit.notes || ''
      }
      if (await this.command('platformAssetSlot', payload)) {
        this.editingSlotId = ''
      }
    },
    releaseName (releaseId) {
      const rel = this.state.releases.find(r => r.id === releaseId)
      return rel ? rel.name : this.t('none')
    },
    personName (id) {
      const p = this.state.participants.find(row => row.id === id)
      return p ? p.name : ''
    },
    taskName (id) {
      const tk = this.state.tasks.find(row => row.id === id)
      return tk ? tk.title : ''
    },
    docById (id) {
      return this.state.documents.find(row => row.id === id)
    },
    docUrl (doc) {
      return '/' + encodeURIComponent(doc.locale) + '/' + doc.path.split('/').map(encodeURIComponent).join('/')
    },
    versionLabel (versionId) {
      const ver = this.state.artifactVersions.find(v => v.id === versionId)
      if (!ver) return versionId
      const art = this.state.artifacts.find(a => a.id === ver.artifactId)
      return (art ? art.name + ' · ' : '') + ver.version
    }
  }
}
</script>

