<template>
  <div style="max-width: 900px; margin: 0 auto">
    <el-card style="margin-bottom: 20px">
      <template #header><h2 style="margin: 0">快速开始</h2></template>
      <el-steps :active="3" finish-status="success" align-center>
        <el-step title="部署管理端" description="启动服务端程序" />
        <el-step title="下载安装包" description="点击生成U盘安装包" />
        <el-step title="安装到目标电脑" description="右键exe以管理员运行" />
        <el-step title="远程管理" description="下发指令、推送升级" />
      </el-steps>
    </el-card>

    <el-row :gutter="20">
      <el-col :span="12">
        <el-card style="margin-bottom: 20px">
          <template #header><h3 style="margin: 0">安装被管理端</h3></template>
          <ol style="line-height: 2; padding-left: 20px">
            <li>在<b>设备管理</b>页面点击<b>生成U盘安装包</b></li>
            <li>将下载的 zip 解压到 U 盘</li>
            <li>在目标电脑上右键 <code>LanAgent.exe</code> → <b>以管理员身份运行</b></li>
            <li>等待显示「[成功]LAN Agent 已安装并启动」</li>
            <li>按回车关闭，安装完成</li>
          </ol>
          <el-alert type="info" :closable="false" style="margin-top: 10px">
            同一个安装包可安装到任意多台设备，每台自动生成唯一标识
          </el-alert>
        </el-card>

        <el-card style="margin-bottom: 20px">
          <template #header><h3 style="margin: 0">卸载被管理端</h3></template>
          <ul style="line-height: 2; padding-left: 20px">
            <li><b>方式一：</b>在设备管理中点击设备的<b>删除</b>按钮（在线设备会自动卸载）</li>
            <li><b>方式二：</b>在目标电脑上右键 <code>uninstall.bat</code> → 以管理员身份运行</li>
          </ul>
          <el-alert type="warning" :closable="false" style="margin-top: 10px">
            卸载会删除服务、程序文件和日志目录
          </el-alert>
        </el-card>
      </el-col>

      <el-col :span="12">
        <el-card style="margin-bottom: 20px">
          <template #header><h3 style="margin: 0">远程操作</h3></template>
          <el-descriptions :column="1" border size="small">
            <el-descriptions-item label="Sysprep 关机">
              执行 <code>sysprep /oobe /shutdown</code>，设备进入 OOBE 体验后关机。<b>不消耗 Rearm 次数</b>
            </el-descriptions-item>
            <el-descriptions-item label="Sysprep 重启">
              执行 <code>sysprep /oobe /quit</code> 后重启，设备进入 OOBE 体验。<b>不消耗 Rearm 次数</b>
            </el-descriptions-item>
            <el-descriptions-item label="批量操作">
              勾选多台设备 → 点击批量按钮 → 确认执行
            </el-descriptions-item>
            <el-descriptions-item label="文件分发">
              上传文件 → 选择目标设备 → 文件保存到 <code>C:\LanAgent\files\</code>
            </el-descriptions-item>
          </el-descriptions>
        </el-card>

        <el-card style="margin-bottom: 20px">
          <template #header><h3 style="margin: 0">推送升级</h3></template>
          <ol style="line-height: 2; padding-left: 20px">
            <li>编译新版 Agent（版本号嵌入 exe）</li>
            <li>更新服务器上的 <code>agent/version.txt</code></li>
            <li>在设备管理页点击<b>推送升级</b></li>
            <li>选择目标版本和目标设备（全部/选中）</li>
            <li>确认后自动推送，Agent 自行下载替换并重启</li>
          </ol>
          <el-alert type="info" :closable="false" style="margin-top: 10px">
            首次安装的旧版 Agent 不支持推送升级，需手动覆盖安装一次新版
          </el-alert>
        </el-card>
      </el-col>
    </el-row>

    <el-card>
      <template #header><h3 style="margin: 0">常见问题</h3></template>
      <el-collapse>
        <el-collapse-item title="安装后管理端看不到设备？">
          <p>1. 确认以管理员身份运行了 LanAgent.exe</p>
          <p>2. 确认目标电脑能访问管理端 IP（检查防火墙）</p>
          <p>3. 等待 5-10 秒，Agent 心跳间隔为 5 秒</p>
        </el-collapse-item>
        <el-collapse-item title="设备显示离线？">
          <p>超过 15 秒未收到心跳会自动标记为离线。可能原因：</p>
          <p>1. 目标电脑关机或断网</p>
          <p>2. Agent 服务被意外停止</p>
          <p>3. 防火墙阻止了出站连接</p>
        </el-collapse-item>
        <el-collapse-item title="推送升级失败？">
          <p>1. 确认目标设备安装的是支持 upgrade 指令的版本（v1.1.0+）</p>
          <p>2. 确认目标设备在线</p>
          <p>3. 查看任务详情中的执行结果获取具体错误信息</p>
        </el-collapse-item>
        <el-collapse-item title="更换管理端服务器IP后设备失联？">
          <p>已安装设备的 deploy.conf 中记录的是旧 IP。需要：</p>
          <p>1. 在新服务器上重新生成安装包</p>
          <p>2. 在目标设备上覆盖安装新包</p>
        </el-collapse-item>
        <el-collapse-item title="Sysprep 和系统准备工具的区别？">
          <p>本系统的 Sysprep 操作等同于 Windows 系统准备工具中选择「进入系统全新体验(OOBE)」+ 关机/重启。</p>
          <p>不带 /generalize 参数，不清除 SID，不消耗 Rearm 次数。</p>
        </el-collapse-item>
      </el-collapse>
    </el-card>
  </div>
</template>

<script setup>
</script>
