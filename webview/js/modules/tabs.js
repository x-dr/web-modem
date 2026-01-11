/* =========================================
   标签切换管理模块 (Tab Management Module)
   ========================================= */

import { $, $$ } from '../utils/dom.js';

/**
 * 标签管理器类
 * 负责管理应用中的标签切换和数据加载
 */
export class TabManager {

    /**
     * 构造函数
     * @param {Object} modemManager - Modem管理器实例
     * @param {Object} smsdbManager - 短信存储管理器实例
     * @param {Object} webhookManager - Webhook管理器实例
     */
    constructor(modemManager, smsdbManager, webhookManager) {
        this.modemManager = modemManager;
        this.smsdbManager = smsdbManager;
        this.webhookManager = webhookManager;
        this.currentTab = 'main';
        // 为所有导航标签绑定点击事件
        $$('.nav-tab').forEach(nav => {
            nav.addEventListener('click', (e) => {
                const tabName = e.target.dataset.tab;
                if (tabName) {
                    this.switchTab(tabName);
                }
            });
        });
    }

    /**
     * 切换标签
     * @param {string} tabName - 要切换到的标签名称
     */
    switchTab(tabName) {
        // 隐藏所有标签内容
        $$('.tab-content').forEach(tab => {
            tab.classList.remove('active');
        });

        // 隐藏所有导航标签
        $$('.nav-tab').forEach(nav => {
            nav.classList.remove('active');
        });

        // 显示选中的标签内容
        const targetTab = $(`#${tabName}Tab`);
        if (targetTab) {
            targetTab.classList.add('active');
        }

        // 激活对应的导航标签
        $$('.nav-tab').forEach((nav, index) => {
            const tabs = ['main', 'smsdb', 'webhook'];
            if (tabs[index] === tabName) {
                nav.classList.add('active');
            }
        });

        this.currentTab = tabName;

        // 根据标签加载相应的数据
        this.loadTabData(tabName);
    }

    /**
     * 加载标签数据
     * 根据当前标签加载相应的数据和设置
     * @param {string} tabName - 标签名称
     */
    loadTabData(tabName) {
        switch (tabName) {
            case 'smsdb':
                if (this.smsdbManager) {
                    this.smsdbManager.loadSmsdbSettings();
                    this.smsdbManager.listSmsdb();
                }
                break;

            case 'webhook':
                if (this.webhookManager) {
                    this.webhookManager.loadWebhookSettings();
                    this.webhookManager.listWebhooks();
                }
                break;

            case 'main':
            default:
                // 主界面不需要特殊处理
                break;
        }
    }

    /**
     * 获取当前标签
     * @returns {string} 当前激活的标签名称
     */
    getCurrentTab() {
        return this.currentTab;
    }
}