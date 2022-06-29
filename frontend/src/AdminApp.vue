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


<style>
html, body {
  margin: 0;
  padding: 0;
}

button {
  padding: 8px;
  border-radius: 8px;
  background: #38F;
  color: white;
  border: 1px solid;
}
</style>