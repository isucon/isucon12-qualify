<template>
  <div class="main">
    <div class="header">
      <HeaderBar />
    </div>
    <div class="body">
      <router-view />
    </div>
  </div>
</template>

<script lang="ts">
import { defineComponent } from 'vue'
import HeaderBar from '@/components/admin/HeaderBar.vue'

export default defineComponent({
  name: 'AdminLayout',
  components: {
    HeaderBar,
  },
})
</script>


<style scoped>
.main {
  display: grid;
  grid-template-columns: 1fr;
  grid-template-rows: 40px 1fr;
}

.header {
}

.body {
}

</style>
