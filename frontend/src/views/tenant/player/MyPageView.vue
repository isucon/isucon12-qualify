<template>
  <div class="mypage">
    <h2>
      大会一覧
    </h2>

    <TableBase
      :header="tableHeader"
      :data="tableData"
      :row-attr="competitions"
    >
      <template #cell-title="slotProps">
         <router-link :to="`/competition/${slotProps.row.id}`">{{ slotProps.row.title }}</router-link>
      </template>
    </TableBase>
  </div>
</template>

<script lang="ts">
import { ref, computed, onMounted, defineComponent } from 'vue'
import axios from 'axios'

import TableBase, { TableColumn } from '@/components/parts/TableBase.vue'

type Competition = {
  id: string
  title: string
  is_finished: boolean
}

export default defineComponent({
  name: 'MyPageView',
  components: {
    TableBase,
  },
  setup() {
    const competitions = ref<Competition[]>([])

    const fetchCompetitions = async () => {
      const res = await axios.get('/api/player/competitions')
      if (!res.data.status) {
        window.alert('failed to fetch competitions: status=false')
        return
      }

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

    const tableHeader: TableColumn[] = [
      {
        width: '20%',
        align: 'center',
        text: '大会ID',
      },
      {
        width: '60%',
        align: 'left',
        text: '大会名',
      },
      {
        width: '20%',
        align: 'center',
        text: '完了ステータス',
      },
    ]

    const tableData = computed<string[][]>(() => 
      competitions.value.map(c => {
        return [
          c.id,
          '##slot##cell-title',
          c.is_finished ? '大会完了' : '開催中',
        ]
      })
    )

    return {
      competitions,
      isLoading,
      noMoreLoad,
      handleLoading,
      tableHeader,
      tableData,
    }
  },
})
</script>

<style scoped>
.mypage {
  padding: 0 20px 20px;
}

</style>
