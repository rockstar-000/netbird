package http

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rs/xid"
	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/management/server"
	"github.com/netbirdio/netbird/management/server/http/api"
	"github.com/netbirdio/netbird/management/server/http/util"
	"github.com/netbirdio/netbird/management/server/jwtclaims"
	"github.com/netbirdio/netbird/management/server/status"
)

// Policies is a handler that returns policy of the account
type Policies struct {
	accountManager  server.AccountManager
	claimsExtractor *jwtclaims.ClaimsExtractor
}

// NewPoliciesHandler creates a new Policies handler
func NewPoliciesHandler(accountManager server.AccountManager, authCfg AuthCfg) *Policies {
	return &Policies{
		accountManager: accountManager,
		claimsExtractor: jwtclaims.NewClaimsExtractor(
			jwtclaims.WithAudience(authCfg.Audience),
			jwtclaims.WithUserIDClaim(authCfg.UserIDClaim),
		),
	}
}

// GetAllPolicies list for the account
func (h *Policies) GetAllPolicies(w http.ResponseWriter, r *http.Request) {
	claims := h.claimsExtractor.FromRequestContext(r)
	account, user, err := h.accountManager.GetAccountFromToken(claims)
	if err != nil {
		util.WriteError(err, w)
		return
	}

	accountPolicies, err := h.accountManager.ListPolicies(account.Id, user.Id)
	if err != nil {
		util.WriteError(err, w)
		return
	}

	util.WriteJSONObject(w, accountPolicies)
}

// UpdatePolicy handles update to a policy identified by a given ID
func (h *Policies) UpdatePolicy(w http.ResponseWriter, r *http.Request) {
	claims := h.claimsExtractor.FromRequestContext(r)
	account, user, err := h.accountManager.GetAccountFromToken(claims)
	if err != nil {
		util.WriteError(err, w)
		return
	}

	vars := mux.Vars(r)
	policyID := vars["policyId"]
	if len(policyID) == 0 {
		util.WriteError(status.Errorf(status.InvalidArgument, "invalid policy ID"), w)
		return
	}

	policyIdx := -1
	for i, policy := range account.Policies {
		if policy.ID == policyID {
			policyIdx = i
			break
		}
	}
	if policyIdx < 0 {
		util.WriteError(status.Errorf(status.NotFound, "couldn't find policy id %s", policyID), w)
		return
	}

	var req api.PutApiPoliciesPolicyIdJSONRequestBody
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		util.WriteErrorResponse("couldn't parse JSON request", http.StatusBadRequest, w)
		return
	}

	if req.Name == "" {
		util.WriteError(status.Errorf(status.InvalidArgument, "policy name shouldn't be empty"), w)
		return
	}

	policy := server.Policy{
		ID:          policyID,
		Name:        req.Name,
		Enabled:     req.Enabled,
		Description: req.Description,
		Query:       req.Query,
	}
	if req.Rules != nil {
		for _, r := range req.Rules {
			pr := server.PolicyRule{
				Destinations: groupMinimumsToStrings(account, r.Destinations),
				Sources:      groupMinimumsToStrings(account, r.Sources),
				Name:         r.Name,
			}
			pr.Enabled = r.Enabled
			if r.Description != nil {
				pr.Description = *r.Description
			}
			if r.Id != nil {
				pr.ID = *r.Id
			}
			switch r.Action {
			case api.PolicyRuleActionAccept:
				pr.Action = server.PolicyTrafficActionAccept
			case api.PolicyRuleActionDrop:
				pr.Action = server.PolicyTrafficActionDrop
			default:
				util.WriteError(status.Errorf(status.InvalidArgument, "unknown action type"), w)
				return
			}
			policy.Rules = append(policy.Rules, &pr)
		}
	}
	if err := policy.UpdateQueryFromRules(); err != nil {
		log.Errorf("failed to update policy query: %v", err)
		util.WriteError(err, w)
		return
	}

	if err = h.accountManager.SavePolicy(account.Id, user.Id, &policy); err != nil {
		util.WriteError(err, w)
		return
	}

	util.WriteJSONObject(w, toPolicyResponse(account, &policy))
}

// CreatePolicy handles policy creation request
func (h *Policies) CreatePolicy(w http.ResponseWriter, r *http.Request) {
	claims := h.claimsExtractor.FromRequestContext(r)
	account, user, err := h.accountManager.GetAccountFromToken(claims)
	if err != nil {
		util.WriteError(err, w)
		return
	}

	var req api.PostApiPoliciesJSONRequestBody
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		util.WriteErrorResponse("couldn't parse JSON request", http.StatusBadRequest, w)
		return
	}

	if req.Name == "" {
		util.WriteError(status.Errorf(status.InvalidArgument, "policy name shouldn't be empty"), w)
		return
	}

	policy := &server.Policy{
		ID:          xid.New().String(),
		Name:        req.Name,
		Enabled:     req.Enabled,
		Description: req.Description,
		Query:       req.Query,
	}

	if req.Rules != nil {
		for _, r := range req.Rules {
			pr := server.PolicyRule{
				ID:           xid.New().String(),
				Destinations: groupMinimumsToStrings(account, r.Destinations),
				Sources:      groupMinimumsToStrings(account, r.Sources),
				Name:         r.Name,
			}
			pr.Enabled = r.Enabled
			if r.Description != nil {
				pr.Description = *r.Description
			}
			switch r.Action {
			case api.PolicyRuleActionAccept:
				pr.Action = server.PolicyTrafficActionAccept
			case api.PolicyRuleActionDrop:
				pr.Action = server.PolicyTrafficActionDrop
			default:
				util.WriteError(status.Errorf(status.InvalidArgument, "unknown action type"), w)
				return
			}
			policy.Rules = append(policy.Rules, &pr)
		}
	}
	if err := policy.UpdateQueryFromRules(); err != nil {
		util.WriteError(err, w)
		return
	}

	if err = h.accountManager.SavePolicy(account.Id, user.Id, policy); err != nil {
		util.WriteError(err, w)
		return
	}

	util.WriteJSONObject(w, toPolicyResponse(account, policy))
}

// DeletePolicy handles policy deletion request
func (h *Policies) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	claims := h.claimsExtractor.FromRequestContext(r)
	account, user, err := h.accountManager.GetAccountFromToken(claims)
	if err != nil {
		util.WriteError(err, w)
		return
	}
	aID := account.Id

	vars := mux.Vars(r)
	policyID := vars["policyId"]
	if len(policyID) == 0 {
		util.WriteError(status.Errorf(status.InvalidArgument, "invalid policy ID"), w)
		return
	}

	if err = h.accountManager.DeletePolicy(aID, policyID, user.Id); err != nil {
		util.WriteError(err, w)
		return
	}

	util.WriteJSONObject(w, emptyObject{})
}

// GetPolicy handles a group Get request identified by ID
func (h *Policies) GetPolicy(w http.ResponseWriter, r *http.Request) {
	claims := h.claimsExtractor.FromRequestContext(r)
	account, user, err := h.accountManager.GetAccountFromToken(claims)
	if err != nil {
		util.WriteError(err, w)
		return
	}

	switch r.Method {
	case http.MethodGet:
		vars := mux.Vars(r)
		policyID := vars["policyId"]
		if len(policyID) == 0 {
			util.WriteError(status.Errorf(status.InvalidArgument, "invalid policy ID"), w)
			return
		}

		policy, err := h.accountManager.GetPolicy(account.Id, policyID, user.Id)
		if err != nil {
			util.WriteError(err, w)
			return
		}

		util.WriteJSONObject(w, toPolicyResponse(account, policy))
	default:
		util.WriteError(status.Errorf(status.NotFound, "method not found"), w)
	}
}

func toPolicyResponse(account *server.Account, policy *server.Policy) *api.Policy {
	cache := make(map[string]api.GroupMinimum)
	ap := &api.Policy{
		Id:          policy.ID,
		Name:        policy.Name,
		Description: policy.Description,
		Enabled:     policy.Enabled,
		Query:       policy.Query,
	}
	if len(policy.Rules) == 0 {
		return ap
	}

	for _, r := range policy.Rules {
		rule := api.PolicyRule{
			Id:          &r.ID,
			Name:        r.Name,
			Enabled:     r.Enabled,
			Description: &r.Description,
		}
		for _, gid := range r.Sources {
			_, ok := cache[gid]
			if ok {
				continue
			}
			if group, ok := account.Groups[gid]; ok {
				minimum := api.GroupMinimum{
					Id:         group.ID,
					Name:       group.Name,
					PeersCount: len(group.Peers),
				}
				rule.Sources = append(rule.Sources, minimum)
				cache[gid] = minimum
			}
		}
		for _, gid := range r.Destinations {
			cachedMinimum, ok := cache[gid]
			if ok {
				rule.Destinations = append(rule.Destinations, cachedMinimum)
				continue
			}
			if group, ok := account.Groups[gid]; ok {
				minimum := api.GroupMinimum{
					Id:         group.ID,
					Name:       group.Name,
					PeersCount: len(group.Peers),
				}
				rule.Destinations = append(rule.Destinations, minimum)
				cache[gid] = minimum
			}
		}
		ap.Rules = append(ap.Rules, rule)
	}
	return ap
}

func groupMinimumsToStrings(account *server.Account, gm []api.GroupMinimum) []string {
	result := make([]string, 0, len(gm))
	for _, gm := range gm {
		if _, ok := account.Groups[gm.Id]; ok {
			continue
		}
		result = append(result, gm.Id)
	}
	return result
}
