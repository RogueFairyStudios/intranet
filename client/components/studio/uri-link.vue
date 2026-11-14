<template>
  <span
    v-if="uri"
    class="studio-uri-row"
  >
    <span class="studio-badge studio-uri-scheme">{{ schemeLabel }}</span>
    <a
      v-if="isWeb"
      :href="uri"
      target="_blank"
      rel="noopener noreferrer"
      class="checksum"
    >{{ compact ? t('openUri') : uri }}</a>
    <a
      v-else-if="isHandler"
      :href="uri"
      class="studio-uri-launch"
    >{{ t('openUri') }} · {{ schemeLabel }}</a>
    <span
      v-if="!compact && !isWeb"
      class="checksum"
    >{{ uri }}</span>
    <button
      type="button"
      class="studio-btn-inline"
      @click="copy"
    >
      {{ copied ? t('copied') : t('copyUri') }}
    </button>
  </span>
</template>
<script>
const handlerSchemes = [
  'aetherion:',
  'goethe:',
  'geppetto:',
  'lore:',
  'lores:',
  'godot:',
  'carnifex:',
  'animafaber:',
  'fontpit:',
  'uefn:',
  'com.epicgames.launcher:'
]

const labels = {
  'aetherion:': 'aetherion://',
  'goethe:': 'goethe://',
  'geppetto:': 'geppetto://',
  'lore:': 'lore://',
  'lores:': 'lores://',
  'godot:': 'godot://',
  'res:': 'res://',
  'uid:': 'uid://',
  'carnifex:': 'carnifex://',
  'animafaber:': 'animafaber://',
  'fontpit:': 'fontpit://',
  'uefn:': 'uefn://',
  'com.epicgames.launcher:': 'Epic / UEFN',
  'https:': 'HTTPS',
  'http:': 'HTTP',
  'file:': 'file://',
  'ssh:': 'SSH',
  'git:': 'Git'
}

export default {
  props: {
    uri: { type: String, default: '' },
    t: { type: Function, required: true },
    compact: Boolean
  },
  data () {
    return { copied: false }
  },
  computed: {
    protocol () {
      if (!this.uri) return ''
      if (/^git@/.test(this.uri)) return 'git:'
      const match = /^([a-zA-Z][a-zA-Z0-9+.-]*:)/.exec(this.uri)
      return match ? match[1].toLowerCase() : ''
    },
    isWeb () {
      return this.protocol === 'https:' || this.protocol === 'http:'
    },
    isHandler () {
      return handlerSchemes.includes(this.protocol)
    },
    schemeLabel () {
      return labels[this.protocol] || this.protocol || 'URI'
    }
  },
  methods: {
    copy () {
      if (navigator && navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(this.uri)
        this.copied = true
        setTimeout(() => { this.copied = false }, 1500)
      }
    }
  }
}
</script>
