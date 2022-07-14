<template>
  <ModalBase
    applyText="作成"
    cancel-text="キャンセル"
    @apply="handleApply"
  >
    <h3>新規大会追加</h3>
    <form>
      <label>大会名</label><input type="text" name="competition_name" v-model="competitionName"/>
    </form>
  </ModalBase>
</template>

<script lang="ts">
import { ref, defineComponent, SetupContext } from 'vue'
import ModalBase from '@/components/parts/ModalBase.vue'

import axios from 'axios'

export default defineComponent({
  name: 'AddCompetitionModal',
  components: {
    ModalBase,
  },
  emits: ['competitionAdded'],
  setup(_, context: SetupContext) {
    const competitionName = ref('')

    const handleApply = async () => {
      const res = await axios.post('/api/organizer/competitions/add', new URLSearchParams({
        title: competitionName.value,
      }))

      if (res.data.status) {
        context.emit('competitionAdded', {
          id: res.data.data.competition.id,
          title: res.data.data.competition.title,
          is_finished:  res.data.data.competition.is_finished,
        })
      }
    }

    return {
      competitionName,
      handleApply,
    }
  },
})
</script>

<style scoped>
h3 {
  color: black;
  margin: 0 0 10px;
}
label {
  color: black;
}
</style>
