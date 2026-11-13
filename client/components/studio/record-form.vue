<template>
  <form @submit.prevent="save">
    <h3>{{ title }}</h3>
    <file-upload
      v-if="enableUpload && projectId"
      :project-id="projectId"
      :busy="busy"
      :t="t"
      @uploaded="applyUpload"
    />
    <label
      v-for="field in fields"
      :key="field.key"
    >
      {{ t(field.label || field.key) }}
      <select
        v-if="field.options"
        v-model="draft[field.key]"
        :multiple="!!field.multiple"
        :required="!!field.required"
      >
        <option
          v-if="!field.multiple && !field.required"
          value=""
        >{{ t('none') }}</option>
        <option
          v-for="option in field.options"
          :key="option.id"
          :value="option.id"
        >{{ option.name }}</option>
      </select>
      <textarea
        v-else-if="field.type === 'textarea'"
        v-model="draft[field.key]"
        :required="!!field.required"
        maxlength="10000"
      />
      <input
        v-else
        v-model="draft[field.key]"
        :type="field.type || 'text'"
        :required="!!field.required"
        :min="field.min"
        :maxlength="field.maxlength || 500"
      >
      <small v-if="field.multiple">{{ t('multiHelp') }}</small>
    </label>
    <button :disabled="busy">
      {{ t('save') }}
    </button>
    <button
      type="button"
      :disabled="busy"
      @click="$emit('cancel')"
    >
      {{ t('cancel') }}
    </button>
  </form>
</template>
<script>
import FileUpload from './file-upload.vue'

export default {
  components: { FileUpload },
  props: {
    title: { type: String, required: true },
    fields: { type: Array, required: true },
    value: { type: Object, required: true },
    busy: Boolean,
    t: { type: Function, required: true },
    submit: { type: Function, required: true },
    projectId: { type: String, default: '' },
    enableUpload: Boolean
  },
  data () {
    return { draft: {} }
  },
  watch: {
    value: {
      immediate: true,
      handler (value) {
        this.draft = JSON.parse(JSON.stringify(value || {}))
        this.fields.forEach(field => {
          if (field.type === 'date' && this.draft[field.key]) {
            this.draft[field.key] = this.draft[field.key].slice(0, 10)
          }
        })
      }
    }
  },
  methods: {
    applyUpload ({ uri, checksum }) {
      this.$set(this.draft, 'uri', uri)
      this.$set(this.draft, 'checksum', checksum)
    },
    async save () {
      const payload = { ...this.draft }
      this.fields.forEach(field => {
        if (field.type === 'number' && payload[field.key] !== '') {
          payload[field.key] = Number(payload[field.key])
        }
      })
      if (await this.submit(payload)) this.$emit('cancel')
    }
  }
}
</script>
