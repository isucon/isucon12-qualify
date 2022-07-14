<template>
  <div class="header-bar">
    <h1 class="brand">
      <img src="/img/isuports_light.svg" alt="ISUPORTS" width="250" height="60"/>
    </h1>
    <h2 class="subtitle">Admin Panel</h2>

    <div class="actions">
      <button
        v-if="isLoggedIn"
        type="button"
        @click="handleLogout"
      >ログアウト</button>
    </div>
  </div>
</template>

<script lang="ts">
import { ref, defineComponent } from 'vue'
import { useRouter } from 'vue-router'
import axios from 'axios'

type Props = {
  isLoggedIn: boolean
}

export default defineComponent({
  props: {
    isLoggedIn: {
      type: Boolean,
      default: false,
    },
  },
  setup() {
    const router = useRouter()

    const handleLogout = async () => {
      try {
        const res = await axios.post('/auth/logout')
        router.push('/')
      } catch (e) {
        window.alert('failed to logout')
      }
    }

    return {
      handleLogout,
    }
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
  vertical-align: middle;
}

.subtitle {
  margin-left: 2em;
  display: inline-block;
  font-size: 20px;
  line-height: 36px;
  vertical-align: middle;
}

.actions {
  float: right;
  margin-top: 12px;
}
</style>