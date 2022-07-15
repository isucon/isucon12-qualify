<template>
  <AdminLayout />
</template>

<script lang="ts">
import { defineComponent } from 'vue'
import AdminLayout from './components/AdminLayout.vue'

export default defineComponent({
  name: 'AdminApp',
  components: {
    AdminLayout,
  },
})
</script>
