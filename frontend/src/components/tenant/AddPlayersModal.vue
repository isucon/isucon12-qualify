<template>
  <ModalBase
    applyText="作成"
    cancel-text="キャンセル"
    @apply="handleApply"
  >
    <h3>プレイヤー追加</h3>

    <h4>追加リスト</h4>
    <table>
      <tr v-for="name in playerNameList" :key="name">
        <td>{{ name }}</td>
      </tr>
    </table>

    <form>
      <input type="text" name="competition_name" v-model="playerName"/>
      <button type="button" class="slim" @click="appendPlayer">リストに追加</button>
    </form>
  </ModalBase>
</template>

<script lang="ts">
import { ref, defineComponent, SetupContext } from 'vue'
import ModalBase from '@/components/parts/ModalBase.vue'

import axios from 'axios'

export default defineComponent({
  name: 'AddPlayersModal',
  components: {
    ModalBase,
  },
  emits: ['playersAdded'],
  setup(_, context: SetupContext) {

    const playerNameList = ref<string[]>([])

    const handleApply = async () => {
      const params = new URLSearchParams()
      playerNameList.value.forEach(name => {
        params.append('display_name[]', name)
      })
      if (playerName.value) {
        params.append('display_name[]', playerName.value)
      }
      const res = await axios.post('/api/organizer/players/add', params)

      if (res.data.status) {
        context.emit('playersAdded')

        playerName.value = ''
        playerNameList.value = []
      }
    }

    const playerName = ref('')
    const appendPlayer = () => {
      playerNameList.value.push(playerName.value)
      playerName.value = ''
    }

    return {
      playerName,
      playerNameList,
      appendPlayer,
      handleApply,
    }
  },
})
</script>

<style scoped>
h3, h4, td {
  color: black;
}

h3 {
  margin: 0 0 10px;
}

table {
  width: 100%;
  border-collapse: collapse;
}

td {
  border: 1px solid lightgray;
}
</style>
