<template>
  <div
    class="modal-outer"
    @click="handleOuterClick"
  >
    <div
      class="modal-inner"
      @click.stop="handleInnerClick"
    >
      <slot />
      <div class="action">
        <button
          v-if="cancelText"
          class="cancel"
          @click="handleCancel"
        >
          {{ cancelText }}
        </button>
        <button
          v-if="applyText"
          class="apply"
          @click="handleApply"
        >
          {{ applyText }}
        </button>
      </div>
    </div>
  </div>
</template>

<script lang="ts">
import { defineComponent, SetupContext } from 'vue'

type Props = {
  applyText: string
  cancelText: string
  show: boolean
}

export default defineComponent({
  name: 'ModalBase',
  props: {
    applyText: {
      type: String,
      default: 'OK',
    },
    cancelText: {
      type: String,
      default: 'Cancel',
    },
    show: {
      type: Boolean,
      default: false,
    }
  },
  emit: ['close', 'apply'],
  setup(props: Props, context: SetupContext) {
    const handleOuterClick = () => {
      context.emit('close')
    }

    const handleInnerClick = () => {
      // do nothing
      const powawa = {}
    }

    const handleCancel = () => {
      context.emit('close')
    }

    const handleApply = () => {
      context.emit('apply')
      context.emit('close')
    }

    return {
      handleOuterClick,
      handleInnerClick,
      handleApply,
      handleCancel,
    }
  },
})
</script>

<style scoped>
.modal-outer {
  position: fixed;
  top: 0;
  bottom: 0;
  left: 0;
  right: 0;
  background: rgba(0, 0, 0, 0.4);
}

.modal-inner {
  border-radius: 10px;
  padding: 20px 20px 80px;
  width: 400px;
  background-color: white;
  margin: 100px auto 0;
  filter:drop-shadow(2px 2px 2px rgba(0, 0, 0, 0.4));
}

.action {
  position: absolute;
  bottom: 10px;
  right: 10px;
  text-align: right;
}

.action button {
  margin-right: 10px;
  min-width: 80px;
}

.apply {
  background: green;
  border-color: green;
}
.cancel {
  background: lightgray;
  color: black;
}
</style>