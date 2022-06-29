<template>
  <div class="players">
    <h2>
      プレイヤー一覧
    </h2>

    <table class="player-list">
      <thead>
        <tr>
          <th class="player-id">プレイヤーID</th>
          <th class="player-name">プレイヤー名</th>
          <th class="player-is-disqualified">参加資格</th>
          <th class="action"></th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="c in players"
          :key="c.id"
        >
          <td class="player-id">{{ c.id }}</td>
          <td class="player-name">{{ c.display_name }}</td>
          <td class="player-is-disqualified">{{ c.is_disqualified ? '失格' : '' }}</td>
          <td class="action"> 失格にするボタン </td>
        </tr>
      </tbody>
    </table>


</div>
</template>

<script lang="ts">
import { ref, onMounted, defineComponent } from 'vue'
import axios from 'axios'

type Player = {
  id: string
  display_name: string
  is_disqualified: boolean
}

export default defineComponent({
  setup() {
    const players = ref<Player[]>([])

    const fetchPlayers = async () => {
      const res = await axios.get('/api/organizer/players')
      if (!res.data.status) {
        window.alert('failed to fetch players: status=false')
        return
      }

      console.log(res.data)
      players.value = res.data.data.players
    }

    onMounted(() => {
      fetchPlayers()
    })

    return {
      players,
    }
  },
})
</script>

<style scoped>
.players {
  padding: 0 20px 20px;
}

.player-list {
  border: 1px solid lightgray;
  border-collapse: collapse;
  width: 100%;
}

th, td {
  padding: 4px;
  border: 1px solid gray;
}

.player-id,
.player-is-disqualified {
  width: 15%;
  text-align: center;
}

.action {
  width: 20%;
  text-align: center;
}

.player-name {
  width: 50%;
}

</style>