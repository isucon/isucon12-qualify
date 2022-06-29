<template>
  <div class="lp">
    <h2>管理画面へのログイン</h2>
    <form
      @submit.prevent="handleSubmit"
    >
      <label for="account">
        アカウント
      </label>
      <input
        v-model="account"
        type="text"
        name="account"
        id="account"
        placeholder="admin"
      />
      <input
        type="submit"
        value="認証する"
      />
    </form>
  </div>
</template>

<script lang="ts">
import { ref, defineComponent } from 'vue'
import { useRouter } from 'vue-router'
import axios from 'axios'

export default defineComponent({
  setup() {
    const router = useRouter()

    const account = ref('')
    const handleSubmit = async () => {
      try {
        const res = await axios.post('/auth/login/admin', new URLSearchParams({
          id: account.value,
        }))

        if (res.status != 200) {
          window.alert('Failed to Login: status=' + res.status)
          return
        }

        router.push('/admin/')

      } catch (e: any) {
        window.alert('Failed to Login: ' + e)
      }
    }

    return {
      account,
      handleSubmit,
    }
  },
})
</script>

<style scoped>
.lp {
  text-align: center;
}
</style>