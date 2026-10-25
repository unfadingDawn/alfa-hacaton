package companies

import (
	"context"

	"github.com/alfainsight/backend/internal/db/dbq"
)

type Service struct {
	Q *dbq.Queries
}

// Load возвращает профиль компании вместе со связями из платёжных данных,
// данных о группе и связях через обезличенных физлиц.
func (s *Service) Load(ctx context.Context, id string, withRelations bool) (Profile, error) {
	c, err := s.Q.GetCompany(ctx, id)
	if err != nil {
		return Profile{}, err
	}
	p := Build(c)
	if !withRelations {
		return p, nil
	}
	rel := &Relations{TopRecipients: []Counterparty{}, TopSenders: []Counterparty{}, Industries: []Industry{},
		GroupMembers: []Counterparty{}, SharedPersons: []Counterparty{}, PersonRegions: []string{}}
	tot, err := s.Q.CompanyPaymentTotals(ctx, id)
	if err != nil {
		return p, err
	}
	rel.OutgoingAmount, rel.OutgoingCount, rel.SupplierCount = tot.OutgoingAmount, tot.OutgoingCount, tot.SupplierCount
	rel.IncomingAmount, rel.IncomingCount, rel.CustomerCount = tot.IncomingAmount, tot.IncomingCount, tot.CustomerCount

	recips, err := s.Q.TopPaymentRecipients(ctx, dbq.TopPaymentRecipientsParams{CompanyID: id, Lim: 8})
	if err != nil {
		return p, err
	}
	for _, r := range recips {
		rel.TopRecipients = append(rel.TopRecipients, Counterparty{ID: r.CounterpartyID, Amount: r.Amount, PayCount: r.PayCount,
			OkvedCode: s2(r.OkvedCode), OkvedName: s2(r.OkvedName), Region: s2(r.Region), Known: r.OkvedCode != nil})
	}
	senders, err := s.Q.TopPaymentSenders(ctx, dbq.TopPaymentSendersParams{CompanyID: id, Lim: 8})
	if err != nil {
		return p, err
	}
	for _, r := range senders {
		rel.TopSenders = append(rel.TopSenders, Counterparty{ID: r.CounterpartyID, Amount: r.Amount, PayCount: r.PayCount,
			OkvedCode: s2(r.OkvedCode), OkvedName: s2(r.OkvedName), Region: s2(r.Region), Known: r.OkvedCode != nil})
	}
	inds, err := s.Q.CounterpartyIndustries(ctx, id)
	if err != nil {
		return p, err
	}
	totals := map[string]float64{}
	for _, in := range inds {
		totals[in.Direction] += in.Amount
	}
	for _, in := range inds {
		share := 0.0
		if totals[in.Direction] > 0 {
			share = in.Amount / totals[in.Direction]
		}
		rel.Industries = append(rel.Industries, Industry{Direction: in.Direction, Division: in.OkvedDivision,
			SampleName: in.SampleOkvedName, Counterparties: in.Counterparties, Amount: in.Amount, Share: share})
	}
	if c.GroupID != nil {
		rel.GroupID = *c.GroupID
		if n, err := s.Q.CountGroupMembers(ctx, c.GroupID); err == nil {
			rel.GroupSize = n
		}
		members, err := s.Q.GroupMembers(ctx, dbq.GroupMembersParams{GroupID: c.GroupID, CompanyID: id, Lim: 10})
		if err == nil {
			for _, m := range members {
				rel.GroupMembers = append(rel.GroupMembers, Counterparty{ID: m.ID, OkvedCode: m.OkvedCode,
					OkvedName: m.OkvedName, Region: s2(m.Region), Known: true, Amount: f2(m.Revenue)})
			}
		}
	}
	if lp, err := s.Q.LinkedPersonsSummary(ctx, id); err == nil {
		rel.LinkedPersons = lp.Persons
		rel.PersonRegions = lp.Regions
	}
	if shared, err := s.Q.CompaniesSharingPersons(ctx, dbq.CompaniesSharingPersonsParams{CompanyID: id, Lim: 10}); err == nil {
		for _, sp := range shared {
			rel.SharedPersons = append(rel.SharedPersons, Counterparty{ID: sp.CompanyID, PayCount: int64(sp.SharedPersons),
				OkvedCode: s2(sp.OkvedCode), OkvedName: s2(sp.OkvedName), Region: s2(sp.Region), Known: sp.OkvedCode != nil})
		}
	}
	p.Relations = rel
	p.Facts = append(p.Facts, p.RelationFacts()...)
	return p, nil
}

func s2(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func f2(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
