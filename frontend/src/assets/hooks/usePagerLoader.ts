import { ref } from 'vue'
import axios from 'axios'

type BasicResponse = {
  data: any
  status: boolean
}

export const usePagerLoader = <T>(args: {
  api: string
  payloadPath: string
  pagerParam: string
  pagerKey: string
  limit: number
}) => {
  const { api, payloadPath, pagerParam, pagerKey, limit } = args
  let nextId = ''
  const noMoreLoad = ref(false)
  const isLoading = ref(false)

  const fetch = async (): Promise<T[]> => {
    const params = new URLSearchParams()
    if (nextId) {
      params.append(pagerParam, nextId)
    }
  
    isLoading.value = true
    try {
      const res = await axios.get(api, { params })
  
      const payload: BasicResponse = res.data
      if (!payload.status) {
        window.alert('Failed to fetch: status=false')
        return []
      }

      const array: T[] = payload.data[payloadPath]
  
      if (!array.length) {
        noMoreLoad.value = true
        return []
      }

      const lastElement: any = array[array.length-1]
      nextId = lastElement[pagerKey]

      if (array.length < limit) {
        noMoreLoad.value = true
      }

      return array
    } catch (e: any) {
      window.alert('Failed to fetch: err=' + e)
    } finally {
      isLoading.value = false
    }
    return []
  }

  return {
    fetch,
    noMoreLoad,
    isLoading,
  }
}
