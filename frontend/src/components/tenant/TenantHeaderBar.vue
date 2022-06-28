<template>
  <div class="header-bar">
    <h1 class="brand">ISUPORTS</h1>
    <h2 class="subtitle">{{ tenantName }}</h2>
  </div>
</template>

<script lang="ts">
import { defineComponent } from 'vue'

type Props = {
  tenantName: string
}

export default defineComponent({
  name: 'TenantHeaderBar',
  props: {
    tenantName: {
      type: String,
      default: '鎌倉ISUPORTS同好会',
    },
  },
})
</script>


<style scoped>
.header-bar {
  text-align: left;
  height: 100%;
  padding: 4px 20px 0;
  max-width: 900px;
  margin: 0 auto;
}

.brand {
  display: inline-block;
  font-size: 38px;
  line-height: 60px;
  margin: 0;
  padding: 0;
}

.subtitle {
  margin-left: 2em;
  display: inline-block;
  font-size: 20px;
  line-height: 36px;
}



</style>