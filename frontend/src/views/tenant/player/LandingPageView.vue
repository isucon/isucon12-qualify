<template>
  <div class="lp">
    <div class="box">
    <h2>プレイヤーサイトへログイン</h2>
    <form
      @submit.prevent="handleSubmit"
    >
      <label for="account">
        アカウントID
      </label>
      <input
        v-model="account"
        type="text"
        name="account"
        id="account"
        placeholder="badcab1e"
      />
      <input
        type="submit"
        value="認証する"
      />
    </form>
    </div>
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
        const res = await axios.post('/auth/login/player', new URLSearchParams({
          id: account.value,
        }))


        if (res.status != 200) {
          window.alert('Failed to Login: status=' + res.status)
          return
        }

        router.push('/mypage')

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

.box {
  border-radius: 4px;
  background-color: white;
  color: #080808;
  padding: 30px 0 60px;
  max-width: 480px;
  margin: 50px auto 0;
}

</style>