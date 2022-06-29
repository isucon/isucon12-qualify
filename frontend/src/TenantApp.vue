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


<style>
html, body {
  margin: 0;
  padding: 0;
  height: 100%;
}

#app {
  height: 100%;
}

button {
  padding: 8px;
  border-radius: 8px;
  background: #38F;
  color: white;
  border: 1px solid;
}
</style>