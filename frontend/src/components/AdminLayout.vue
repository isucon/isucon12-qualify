<template>
  <div class="main light">
    <div class="header">
      <AdminHeaderBar
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
import { useRouter } from 'vue-router'
import { useLoginStatus } from '@/assets/hooks/useLoginStatus'

import AdminHeaderBar from '@/components/admin/AdminHeaderBar.vue'

export default defineComponent({
  name: 'AdminLayout',
  components: {
    AdminHeaderBar,
  },
  setup() {
    const { isLoggedIn, refetch } = useLoginStatus()

    const router = useRouter()
    router.afterEach(async (to, from) => {
      await refetch()

      // check login status
      checkAndRedirect(to.fullPath)
    })

    const checkAndRedirect = (fullPath: string) => {
      if (isLoggedIn.value) {
        if (fullPath === '/') {
          router.push('/admin/')
        }
      } else {
        if (fullPath !== '/') {
          router.push('/')
        }
      }
    }


    return {
      isLoggedIn,
    }
  },
})
</script>


<style scoped>
.main {
  color: #222;
  height: 100%;
}

.main:after {
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  content: "";
  background: linear-gradient(#FFFFFF, #FFFFFF, #fff8f5);
  z-index: -1;
}

.header {
  background: orange;
  color: white;
  filter: drop-shadow(0px 2px 3px rgba(0, 0, 0, 0.4));
  position: sticky;
  height: 70px;
  top: 0;
  left: 0;
  right: 0;
}

.body {
  padding-top: 10px;
  max-width: 940px;
  margin: 20px auto 0;
}

</style>
