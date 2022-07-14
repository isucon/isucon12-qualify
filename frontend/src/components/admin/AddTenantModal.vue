<template>
  <ModalBase
    applyText="作成"
    cancel-text="キャンセル"
    @apply="handleApply"
  >
    <h3>新規テナント追加</h3>
    <form>
      <label>テナント名(サブドメイン)</label><input type="text" name="tenant_name" v-model="tenantName"/>
      <label>テナント表示名</label><input type="text" name="tenant_display_name" v-model="tenantDisplayName"/>
    </form>
  </ModalBase>
</template>

<script lang="ts">
import { ref, defineComponent, SetupContext } from 'vue'
import ModalBase from '@/components/parts/ModalBase.vue'

import axios from 'axios'

export default defineComponent({
  name: 'AddTenantModal',
  components: {
    ModalBase,
  },
  emits: ['tenantAdded'],
  setup(_, context: SetupContext) {
    const tenantName = ref('')
    const tenantDisplayName = ref('')


    const handleApply = async () => {
      const res = await axios.post('/api/admin/tenants/add', new URLSearchParams({
        name: tenantName.value,
        display_name: tenantDisplayName.value,
      }))

      if (res.data.status) {
        context.emit('tenantAdded', {
          id: 'x',
          name: res.data.data.tenant.name,
          display_name: res.data.data.tenant.display_name,
          billing: 0
        })
      }
    }

    return {
      tenantName,
      tenantDisplayName,
      handleApply,
    }
  },
})
</script>

<style scoped>
h3 {
  margin: 0 0 10px;
}
</style>
