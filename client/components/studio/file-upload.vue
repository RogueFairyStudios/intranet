<template>
  <div class="studio-upload">
    <label>{{ t('uploadFile') }}<input
      ref="fileInput"
      type="file"
      :disabled="busy || uploading"
      @change="onSelect"
    ></label>
    <p
      v-if="uploading"
      role="status"
      class="studio-muted"
    >
      {{ t('uploading') }}
    </p>
    <p
      v-if="status"
      role="status"
      class="studio-muted"
    >
      {{ status }}
    </p>
    <p
      v-if="uploadError"
      role="alert"
      class="studio-error"
    >
      {{ uploadError }}
    </p>
  </div>
</template>
<script>
import Cookies from 'js-cookie'

export default {
  props: {
    projectId: { type: String, required: true },
    busy: Boolean,
    t: { type: Function, required: true }
  },
  data () {
    return { uploading: false, status: '', uploadError: '' }
  },
  methods: {
    async onSelect (event) {
      const file = event.target.files && event.target.files[0]
      if (!file) return
      this.uploading = true
      this.status = ''
      this.uploadError = ''
      try {
        const form = new FormData()
        form.append('file', file)
        const headers = {}
        const jwtToken = Cookies.get('jwt')
        if (jwtToken) headers.Authorization = `Bearer ${jwtToken}`
        const resp = await fetch(`/p/studio/files/${encodeURIComponent(this.projectId)}`, {
          method: 'POST',
          headers,
          credentials: 'same-origin',
          body: form
        })
        if (!resp.ok) {
          let message = `HTTP ${resp.status}`
          try {
            const payload = await resp.json()
            if (payload && payload.error) message = payload.error
          } catch (err) {
            // keep HTTP status fallback
          }
          throw new Error(message)
        }
        const result = await resp.json()
        this.$emit('uploaded', {
          uri: window.location.origin + result.path,
          checksum: result.checksum,
          name: file.name
        })
        this.status = this.t('uploaded')
      } catch (error) {
        this.uploadError = error.message
      } finally {
        this.uploading = false
        if (this.$refs.fileInput) this.$refs.fileInput.value = ''
      }
    }
  }
}
</script>
