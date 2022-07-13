<template>
  <div class="main dark">
    <div class="header">
      <TenantHeaderBar
        :tenant-name="tenantDisplayName"
        :is-logged-in="isLoggedIn"
      />
    </div>
    <div class="body">
      <router-view />
    </div>
  </div>
</template>

<script lang="ts">
import { defineComponent } from 'vue'
import TenantHeaderBar from '@/components/tenant/TenantHeaderBar.vue'

import { useLoginStatus } from '@/assets/hooks/useLoginStatus'

import axios from 'axios'

export default defineComponent({
  name: 'TenantLayout',
  components: {
    TenantHeaderBar,
  },
  setup() {
    const { isLoggedIn, tenantDisplayName } = useLoginStatus()

    return {
      tenantDisplayName,
      isLoggedIn,
    }
  },
})
</script>


<style scoped>
.main {
  color: #F8F8FF;
  height: 100%;
}

.main:after {
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  content: "";
  background: linear-gradient(#301855,#301855, #0a0833);
  z-index: -1;
}


.header {
  background: rgb(16, 11, 27);
  color: white;
  filter:drop-shadow(0px 2px 3px rgba(0, 0, 0, 0.4));
  position: sticky;
  height: 70px;
  top: 0;
  left: 0;
  right: 0;
}

.body {
  max-width: 940px;
  margin: 20px auto 0;
}

</style>
