package agent

// Note: the legacy UpdateRTPEngineInterfaces / UpdateKamailioListen helpers
// (which mutated a single line in an existing config) have been replaced
// by GenRTPEngineConfig / GenKamailioConfig which produce a complete
// authoritative file each time. See rtpengine.go and kamailio.go.
