<template>
  <div class="organizer-competition-list">
    <h2>
      大会一覧
    </h2>

    <table class="competition-list">
      <thead>
        <tr>
          <th class="competition-id">大会ID</th>
          <th class="competition-name">大会名</th>
          <th class="competition-display-name">完了ステータス</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="c in competitions"
          :key="c.id"
        >
          <td class="competition-id">{{ c.id }}</td>
          <td class="competition-title">{{ c.title }}</td>
          <td class="finish-status">{{ c.is_finished ? '大会完了' : '開催中' }}</td>
        </tr>
      </tbody>
    </table>


</div>
</template>

<script lang="ts">
import { ref, onMounted, defineComponent } from 'vue'
import axios from 'axios'

type Competition = {
  id: string
  title: string
  is_finished: boolean
}

export default defineComponent({
  name: 'CompetitionListView',
  setup() {
    const competitions = ref<Competition[]>([])

    const fetchCompetitions = async () => {
      const res = await axios.get('/api/player/competitions')
      if (!res.data.status) {
        window.alert('failed to fetch competitions: status=false')
        return
      }

      console.log(res.data)
      competitions.value = res.data.data.competitions
    }

    onMounted(() => {
      fetchCompetitions()
    })

    const handleLoading = () => {
      return
    }

    const isLoading = ref(false)
    const noMoreLoad = ref(false)

    return {
      competitions,
      isLoading,
      noMoreLoad,
      handleLoading,
    }
  },
})
</script>

<style scoped>
.organizer-competition-list {
  padding: 0 20px 20px;
}

.competition-list {
  border: 1px solid lightgray;
  border-collapse: collapse;
  width: 100%;
}

th, td {
  padding: 4px;
  border: 1px solid gray;
}

.competition-id,
.finish-status {
  width: 20%;
  text-align: center;
}

.competition-title {
  width: 60%;
}


@keyframes rotation {
  0%{ transform:rotate(0);}
  100%{ transform:rotate(360deg); }
}

.cycle-loop {
  display: inline-block;
  animation: 1s linear infinite rotation;
}

.loading {
  text-align: center;
  height: 40px;
}


</style>
