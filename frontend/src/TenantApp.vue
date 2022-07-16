<template>
  <TenantLayout />
</template>

<script lang="ts">
import { defineComponent } from 'vue'
import TenantLayout from './components/TenantLayout.vue'

export default defineComponent({
  name: 'TenantApp',
  components: {
    TenantLayout,
  },
})
</script>
