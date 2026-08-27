package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/iniwex5/vohive/internal/db"
	"github.com/iniwex5/vohive/internal/voicecall"
	"github.com/iniwex5/vohive/pkg/logger"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域及本地访问
	},
}

// handleVoiceStatus 获取当前呼叫状态
func (s *Server) handleVoiceStatus(c *gin.Context) {
	if s.voiceCallMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "语音呼叫模块未就绪"})
		return
	}
	c.JSON(http.StatusOK, s.voiceCallMgr.GetStatus())
}

// handleVoiceDial 发起呼叫
func (s *Server) handleVoiceDial(c *gin.Context) {
	if s.voiceCallMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "语音呼叫模块未就绪"})
		return
	}

	var req voicecall.DialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的拨号请求参数"})
		return
	}

	session, err := s.voiceCallMgr.Dial(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, session)
}

// handleVoiceAnswer 接听来电
func (s *Server) handleVoiceAnswer(c *gin.Context) {
	if s.voiceCallMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "语音呼叫模块未就绪"})
		return
	}

	session, err := s.voiceCallMgr.Answer(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, session)
}

// handleVoiceHangup 挂断呼叫
func (s *Server) handleVoiceHangup(c *gin.Context) {
	if s.voiceCallMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "语音呼叫模块未就绪"})
		return
	}

	if err := s.voiceCallMgr.Hangup(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// handleVoiceDTMF 发送按键音
func (s *Server) handleVoiceDTMF(c *gin.Context) {
	if s.voiceCallMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "语音呼叫模块未就绪"})
		return
	}

	var req voicecall.DTMFRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的按键参数"})
		return
	}

	if err := s.voiceCallMgr.SendDTMF(c.Request.Context(), req.Digit); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// handleVoiceWebRTCOffer 处理前端 WebRTC SDP Offer
func (s *Server) handleVoiceWebRTCOffer(c *gin.Context) {
	if s.voiceCallMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "语音呼叫模块未就绪"})
		return
	}

	var req voicecall.WebRTCOfferRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.SDP == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "SDP offer 不能为空"})
		return
	}

	answerSDP, err := s.voiceCallMgr.HandleWebRTCOffer(c.Request.Context(), req.SDP)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, voicecall.WebRTCAnswerResponse{SDP: answerSDP})
}

// handleVoiceWebRTCCandidate 处理前端 WebRTC ICE Candidate
func (s *Server) handleVoiceWebRTCCandidate(c *gin.Context) {
	if s.voiceCallMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "语音呼叫模块未就绪"})
		return
	}

	var req voicecall.WebRTCCandidateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 candidate 参数"})
		return
	}

	if err := s.voiceCallMgr.HandleICECandidate(req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// handleVoiceWS WebSocket 实时呼叫事件与状态推送
func (s *Server) handleVoiceWS(c *gin.Context) {
	if s.voiceCallMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "语音呼叫模块未就绪"})
		return
	}

	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Warn("升级 WebSocket 失败", "err", err)
		return
	}
	defer ws.Close()

	ch, unsubscribe := s.voiceCallMgr.SubscribeEvents()
	defer unsubscribe()

	// 心跳保活
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, _, err := ws.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		case <-pingTicker.C:
			if err := ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if err := ws.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}
	}
}

// handleVoiceRecords 获取通话历史记录列表
func (s *Server) handleVoiceRecords(c *gin.Context) {
	deviceID := c.Query("device_id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	records, total, err := db.GetCallRecords(deviceID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"records": records,
		"total":   total,
	})
}

// handleVoiceTopContacts 获取近期最高频通话的前 3 个联系人
func (s *Server) handleVoiceTopContacts(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "3"))
	contacts, err := db.GetTopCallContacts(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"contacts": contacts,
	})
}

// handleVoiceDeleteRecord 删除单条通话记录
func (s *Server) handleVoiceDeleteRecord(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的记录 ID"})
		return
	}
	if err := db.DeleteCallRecord(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// handleVoiceClearRecords 清空所有通话记录
func (s *Server) handleVoiceClearRecords(c *gin.Context) {
	if err := db.ClearCallRecords(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
