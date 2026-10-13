<script setup lang="ts">
import { h, onMounted, ref, computed } from "vue";
import { NButton, NPopconfirm, useMessage } from "naive-ui";
import type { DataTableColumns } from "naive-ui";
import { fetchListMemories, fetchDeleteMemory } from "@/service/api";
import type { Mem0Memory } from "@/service/api/memory";

const message = useMessage();
const loading = ref(false);
const data = ref<Mem0Memory[]>([]);
const keyword = ref("");

const columns: DataTableColumns<Mem0Memory> = [
  {
    title: "记忆内容",
    key: "memory",
    minWidth: 400,
    ellipsis: {
      tooltip: true,
    },
  },
  {
    title: "创建时间",
    key: "created_at",
    width: 200,
    render(row) {
      if (!row.created_at) return h("span", "-");
      return h("span", new Date(row.created_at).toLocaleString());
    },
  },
  {
    title: "操作",
    key: "actions",
    width: 120,
    fixed: "right",
    render(row) {
      return h(
        NPopconfirm,
        {
          onPositiveClick: () => handleDelete(row.id),
          trigger: "click",
        },
        {
          trigger: () =>
            h(
              NButton,
              { size: "small", type: "error", quaternary: true },
              { default: () => "删除" }
            ),
          default: () => "确认删除这条记忆？",
        }
      );
    },
  },
];

const filteredData = computed(() => {
  if (!keyword.value.trim()) return data.value;
  const kw = keyword.value.toLowerCase();
  return data.value.filter((m) => m.memory.toLowerCase().includes(kw));
});

const loadData = async () => {
  loading.value = true;
  try {
    const { data: res } = await fetchListMemories();
    if (res) {
      data.value = res;
    }
  } catch (err: any) {
    message.error(`加载记忆失败: ${err?.message || "未知错误"}`);
  } finally {
    loading.value = false;
  }
};

const handleDelete = async (id: string) => {
  try {
    await fetchDeleteMemory(id);
    message.success("记忆已删除");
    loadData();
  } catch (err: any) {
    message.error(`删除失败: ${err?.message || "未知错误"}`);
  }
};

onMounted(() => {
  loadData();
});
</script>

<template>
  <div class="h-full flex-col flex gap-4 p-4">
    <NCard :bordered="false" shadow="sm" class="flex-1">
      <template #header>
        <div class="flex items-center gap-4">
          <span class="text-18px font-bold">记忆管理</span>
          <NTag type="info" size="small">{{ data.length }}条记忆</NTag>
        </div>
      </template>
      <div class="flex flex-col h-full gap-4">
        <div class="flex justify-between items-center">
          <NInput
            v-model:value="keyword"
            placeholder="搜索记忆内容..."
            clearable
            style="width: 300px"
          />
          <ButtonIcon icon="mdi:refresh" tooltip-content="刷新" @click="loadData" />
        </div>

        <NDataTable
          :columns="columns"
          :data="filteredData"
          :loading="loading"
          :pagination="{ pageSize: 15 }"
          :row-key="(row) => row.id"
          flex-height
          class="flex-1"
        />
      </div>
    </NCard>
  </div>
</template>

<style scoped></style>
