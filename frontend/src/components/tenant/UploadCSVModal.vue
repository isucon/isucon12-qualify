<template>
  <ModalBase
    applyText="アップロード"
    cancel-text="キャンセル"
    @apply="handleApply"
    @close="handleClose"
  >
    <h3>スコアアップロード</h3>
    <p>大会名: {{ competitionTitle }} にスコアCSVを登録します。</p>
    <form>
      <label>CSVファイルを選択</label>
      <input ref="inputRef" type="file" accept="text/csv" name="score" @change="handleFileChange"/>
    </form>
  </ModalBase>
</template>

<script lang="ts">
import { ref, watch, toRef, defineComponent, SetupContext } from 'vue'
import ModalBase from '@/components/parts/ModalBase.vue'

import axios from 'axios'

type Props = {
  competitionId: string
  competitionTitle: string
}

export default defineComponent({
  name: 'UploadCSVModal',
  components: {
    ModalBase,
  },
  props: {
    competitionId: {
      type: String,
      required: true,
    },
    competitionTitle: {
      type: String,
      required: true,
    },
  },
  emits: ['csvUploaded'],
  setup(props: Props, context: SetupContext) {
    const competitionId = ref(props.competitionId)
    const refCompetitionId = toRef(props, 'competitionId')
    watch(refCompetitionId, (newVal) => {
      competitionId.value = newVal
    })

    const competitionTitle = ref(props.competitionTitle)
    const refCompetitionTitle = toRef(props, 'competitionTitle')
    watch(refCompetitionTitle, (newVal) => {
      competitionTitle.value = newVal
    })

    const handleApply = async () => {
      const params = new FormData()
      if (!csvFile.value) return
      params.append('scores', csvFile.value)

      const res = await axios.post(
        `/api/organizer/competition/${competitionId.value}/score`,
        params,
        {
          headers: {
            'content-type': 'multipart/form-data',
          },
        }
      )

      if (res.data.status) {
        context.emit('csvUploaded', {
          uploaded: res.data.data.rows,
        })
      }
    }

    const csvFile = ref<File | undefined>()
    const handleFileChange = (e: Event) => {
      const input = e.target as HTMLInputElement
      const file = input.files ? input.files[0] : undefined
      csvFile.value = file
    }

    const inputRef = ref<HTMLInputElement|null>(null)
    const handleClose = () => {
      if (!inputRef.value) return
      inputRef.value.value = ''
    }

    return {
      inputRef,
      handleApply,
      handleFileChange,
      handleClose,
    }
  },
})
</script>

<style scoped>
h3 {
  color: black;
  margin: 0 0 10px;
}
p, label, input {
  color: black;
}

input {
  width: 100%
}

</style>
