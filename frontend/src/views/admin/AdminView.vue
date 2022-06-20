<template>
  <div class="admin">
    アドみん
  </div>
</template>

<script lang="ts">
import { onMounted, defineComponent } from 'vue'
import axios from 'axios'

export default defineComponent({
  setup() {

    onMounted(async () => {
      const billing = await axios.get('/api/admin/tenants/billing')

      console.log(billing.data)

    })


    return {}
  },
})
</script>

