import { ref, onMounted } from 'vue'
import axios from 'axios'

type MeResponse = {
  data: {
    tenant: {
      name: string
      display_name: string
    }
    loggedIn: boolean
  }
  status: boolean
}

export const useLoginStatus = () => {
  const isLoggedIn = ref(false)
  const tenantDisplayName = ref('')

  const fetch = async () => {
    const res = await axios.get('/api/me')
    const payload: MeResponse = res.data

    isLoggedIn.value = payload.data.loggedIn
    tenantDisplayName.value = payload.data.tenant.display_name
  }

  onMounted(() => {
    fetch()
  })

  const refetch = () => {
    fetch()
  }

  return {
    tenantDisplayName,
    isLoggedIn,
    refetch,
  }
}
