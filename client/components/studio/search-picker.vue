<template>
  <fieldset>
    <legend>{{ label }}</legend>
    <label>{{ t('search') }}<input
      v-model="search"
      type="search"
      @input="schedule"
    ></label>
    <p
      v-if="error"
      role="alert"
    >
      {{ error }}
    </p>
    <p
      v-if="loading"
      role="status"
    >
      {{ t('loading') }}
    </p>
    <label>{{ t('results') }}<select
      :value="value || ''"
      :required="required"
      @change="choose"
    >
      <option value="">{{ t('none') }}</option>
      <option
        v-if="value && !results.some(item => item.id === value)"
        :value="value"
      >{{ t('currentSelection') }}</option>
      <option
        v-for="item in results"
        :key="item.id"
        :value="item.id"
      >{{ item.name || item.title }}{{ item.path ? ' / ' + item.path : '' }}</option>
    </select></label>
    <p v-if="searched && !loading && !results.length">
      {{ t('noResults') }}
    </p>
  </fieldset>
</template>
<script>
import gql from 'graphql-tag'

const queries = {
  pages: gql`
    query ($projectId: String!, $search: String!) {
      studioPages(projectId: $projectId, search: $search) {
        id
        title
        path
        locale
      }
    }
  `,
  groups: gql`
    query ($projectId: String!, $search: String!) {
      studioGroups(projectId: $projectId, search: $search) {
        id
        name
      }
    }
  `,
  users: gql`
    query ($projectId: String!, $search: String!) {
      studioUsers(projectId: $projectId, search: $search) {
        id
        name
      }
    }
  `
}

export default {
  props: {
    value: { type: [String, Number], default: null },
    projectId: { type: String, required: true },
    kind: { type: String, required: true },
    label: { type: String, required: true },
    t: { type: Function, required: true },
    required: Boolean
  },
  data () {
    return { search: '', results: [], loading: false, error: '', timer: null, sequence: 0, searched: false }
  },
  watch: {
    projectId () {
      this.search = ''
      this.find()
    }
  },
  mounted () {
    this.find()
  },
  beforeDestroy () {
    clearTimeout(this.timer)
    this.sequence++
  },
  methods: {
    schedule () {
      clearTimeout(this.timer)
      this.timer = setTimeout(this.find, 300)
    },
    choose (event) {
      this.$emit('input', event.target.value ? Number(event.target.value) : null)
    },
    async find () {
      if (!this.projectId) return
      const sequence = ++this.sequence
      this.loading = true
      this.error = ''
      const name = this.kind === 'pages' ? 'studioPages' : this.kind === 'groups' ? 'studioGroups' : 'studioUsers'
      const query = queries[this.kind] || queries.users
      try {
        const { data } = await this.$apollo.query({
          query,
          variables: { projectId: this.projectId, search: this.search },
          fetchPolicy: 'network-only'
        })
        if (sequence === this.sequence) {
          this.results = data[name]
          this.searched = true
        }
      } catch (error) {
        if (sequence === this.sequence) this.error = error.message
      } finally {
        if (sequence === this.sequence) this.loading = false
      }
    }
  }
}
</script>
