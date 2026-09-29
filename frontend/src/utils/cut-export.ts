/**
 * 切割图导出公共工具 (canvas 转 Blob / 触发下载 / 时间戳文件名)
 */

export function formatFileTimestamp(): string {
  const now = new Date();
  const pad = (n: number) => String(n).padStart(2, '0');
  return (
    `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}` +
    `-${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`
  );
}

export function canvasToBlob(canvas: HTMLCanvasElement): Promise<Blob> {
  return new Promise((resolve, reject) => {
    if (typeof canvas.toBlob === 'function') {
      canvas.toBlob(blob => {
        if (blob) {
          resolve(blob);
        } else {
          reject(new Error('canvas.toBlob returned null'));
        }
      }, 'image/png');
      return;
    }
    // 兼容回退: dataURL 转 Blob
    try {
      const dataUrl = canvas.toDataURL('image/png');
      const base64 = dataUrl.split(',')[1] || '';
      const binStr = atob(base64);
      const bytes = new Uint8Array(binStr.length);
      for (let i = 0; i < binStr.length; i += 1) {
        bytes[i] = binStr.charCodeAt(i);
      }
      resolve(new Blob([bytes], { type: 'image/png' }));
    } catch (e) {
      reject(e instanceof Error ? e : new Error('canvas.toDataURL failed'));
    }
  });
}

export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  // 延迟释放, 避免部分浏览器下载未开始就 revoke
  setTimeout(() => {
    URL.revokeObjectURL(url);
  }, 1000);
}

/**
 * 打开 PDF 用于打印 (配合 jsPDF autoPrint, 新窗口打开后浏览器自动弹出打印对话框)
 *
 * @returns 是否成功打开; 新窗口被拦截时回退隐藏 iframe 打印, 仍失败返回 false
 */
export function printBlobUrl(url: string): boolean {
  const win = window.open(url, '_blank');
  if (win) return true;

  // 弹窗被拦截: 隐藏 iframe 兜底打印
  try {
    const iframe = document.createElement('iframe');
    iframe.style.position = 'fixed';
    iframe.style.right = '0';
    iframe.style.bottom = '0';
    iframe.style.width = '1px';
    iframe.style.height = '1px';
    iframe.style.opacity = '0';
    iframe.style.border = 'none';
    iframe.src = url;
    document.body.appendChild(iframe);
    const printWindow = iframe.contentWindow;
    if (!printWindow) {
      document.body.removeChild(iframe);
      return false;
    }
    window.setTimeout(() => {
      printWindow.focus();
      printWindow.print();
    }, 300);
    return true;
  } catch {
    return false;
  }
}
