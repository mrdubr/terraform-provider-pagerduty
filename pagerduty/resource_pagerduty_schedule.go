package pagerduty

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/heimweh/go-pagerduty/pagerduty"
)

func resourcePagerDutySchedule() *schema.Resource {
	return &schema.Resource{
		Create: resourcePagerDutyScheduleCreate,
		Read:   resourcePagerDutyScheduleRead,
		Update: resourcePagerDutyScheduleUpdate,
		Delete: resourcePagerDutyScheduleDelete,
		CustomizeDiff: func(context context.Context, diff *schema.ResourceDiff, i interface{}) error {
			ln := diff.Get("layer.#").(int)
			for li := 0; li <= ln; li++ {
				rn := diff.Get(fmt.Sprintf("layer.%d.restriction.#", li)).(int)
				for ri := 0; ri <= rn; ri++ {
					t := diff.Get(fmt.Sprintf("layer.%d.restriction.%d.type", li, ri)).(string)
					if t == "daily_restriction" && diff.Get(fmt.Sprintf("layer.%d.restriction.%d.start_day_of_week", li, ri)).(int) != 0 {
						return fmt.Errorf("start_day_of_week must only be set for a weekly_restriction schedule restriction type")
					}
					ds := diff.Get(fmt.Sprintf("layer.%d.restriction.%d.duration_seconds", li, ri)).(int)
					if t == "daily_restriction" && ds >= 3600*24 {
						return fmt.Errorf("duration_seconds for a daily_restriction schedule restriction type must be shorter than a day")
					}
				}
			}
			return nil
		},
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"time_zone": {
				Type:     schema.TypeString,
				Required: true,
				ValidateFunc: func(val interface{}, key string) (warns []string, errs []error) {
					_, err := time.LoadLocation(val.(string))
					if err != nil {
						errs = append(errs, err)
					}
					return
				},
			},

			"overflow": {
				Type:     schema.TypeBool,
				Optional: true,
			},

			"description": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "Managed by Terraform",
			},

			"layer": {
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:     schema.TypeString,
							Computed: true,
						},

						"name": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},

						"start": {
							Type:             schema.TypeString,
							Required:         true,
							ValidateFunc:     validateRFC3339,
							DiffSuppressFunc: suppressScheduleLayerStartDiff,
						},

						"end": {
							Type:             schema.TypeString,
							Optional:         true,
							ValidateFunc:     validateRFC3339,
							DiffSuppressFunc: suppressRFC3339Diff,
						},

						"rotation_virtual_start": {
							Type:             schema.TypeString,
							Required:         true,
							ValidateFunc:     validateRFC3339,
							DiffSuppressFunc: suppressRFC3339Diff,
						},

						"rotation_turn_length_seconds": {
							Type:         schema.TypeInt,
							Required:     true,
							ValidateFunc: validation.IntBetween(3600, 365*24*3600),
						},

						"users": {
							Type:     schema.TypeList,
							Required: true,
							MinItems: 1,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},

						"rendered_coverage_percentage": {
							Type:     schema.TypeString,
							Computed: true,
						},

						"restriction": {
							Optional: true,
							Type:     schema.TypeList,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"type": {
										Type:     schema.TypeString,
										Required: true,
										ValidateFunc: validateValueFunc([]string{
											"daily_restriction",
											"weekly_restriction",
										}),
									},

									"start_time_of_day": {
										Type:         schema.TypeString,
										Required:     true,
										ValidateFunc: validation.StringMatch(regexp.MustCompile(`([0-1][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9]`), "must be of 00:00:00 format"),
									},

									"start_day_of_week": {
										Type:         schema.TypeInt,
										Optional:     true,
										ValidateFunc: validation.IntBetween(1, 7),
									},

									"duration_seconds": {
										Type:         schema.TypeInt,
										Required:     true,
										ValidateFunc: validation.IntBetween(1, 7*24*3600-1),
									},
								},
							},
						},
					},
				},
			},
			"teams": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},

			"final_schedule": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"rendered_coverage_percentage": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func buildScheduleStruct(d *schema.ResourceData) (*pagerduty.Schedule, error) {
	layers, err := expandScheduleLayers(d.Get("layer"))
	if err != nil {
		return nil, err
	}

	schedule := &pagerduty.Schedule{
		Name:           d.Get("name").(string),
		TimeZone:       d.Get("time_zone").(string),
		ScheduleLayers: layers,
	}

	if attr, ok := d.GetOk("description"); ok {
		schedule.Description = attr.(string)
	}

	if attr, ok := d.GetOk("teams"); ok {
		schedule.Teams = expandSchedTeams(attr.([]interface{}))
	}

	return schedule, nil
}

func resourcePagerDutyScheduleCreate(d *schema.ResourceData, meta interface{}) error {
	client, err := meta.(*Config).Client()
	if err != nil {
		return err
	}

	schedule, err := buildScheduleStruct(d)
	if err != nil {
		return err
	}

	o := &pagerduty.CreateScheduleOptions{}

	if v, ok := d.GetOk("overflow"); ok {
		o.Overflow = v.(bool)
	}

	log.Printf("[INFO] Creating PagerDuty schedule: %s", schedule.Name)

	schedule, _, err = client.Schedules.Create(schedule, o)
	if err != nil {
		return err
	}

	d.SetId(schedule.ID)

	return resourcePagerDutyScheduleRead(d, meta)
}

func resourcePagerDutyScheduleRead(d *schema.ResourceData, meta interface{}) error {
	client, err := meta.(*Config).Client()
	if err != nil {
		return err
	}

	log.Printf("[INFO] Reading PagerDuty schedule: %s", d.Id())

	retryErr := resource.Retry(30*time.Second, func() *resource.RetryError {
		if schedule, _, err := client.Schedules.Get(d.Id(), &pagerduty.GetScheduleOptions{}); err != nil {
			time.Sleep(2 * time.Second)
			return resource.RetryableError(err)
		} else if schedule != nil {
			d.Set("name", schedule.Name)
			d.Set("time_zone", schedule.TimeZone)
			d.Set("description", schedule.Description)

			layers, err := flattenScheduleLayers(schedule.ScheduleLayers)
			if err != nil {
				return resource.NonRetryableError(err)
			}

			if err := d.Set("layer", layers); err != nil {
				return resource.NonRetryableError(err)
			}
			if err := d.Set("teams", flattenShedTeams(schedule.Teams)); err != nil {
				return resource.NonRetryableError(fmt.Errorf("error setting teams: %s", err))
			}
			if err := d.Set("final_schedule", flattenScheFinalSchedule(schedule.FinalSchedule)); err != nil {
				return resource.NonRetryableError(fmt.Errorf("error setting final_schedule: %s", err))
			}

		}
		return nil
	})

	if retryErr != nil {
		time.Sleep(2 * time.Second)
		return retryErr
	}

	return nil
}

func resourcePagerDutyScheduleUpdate(d *schema.ResourceData, meta interface{}) error {
	client, err := meta.(*Config).Client()
	if err != nil {
		return err
	}

	schedule, err := buildScheduleStruct(d)
	if err != nil {
		return err
	}

	opts := &pagerduty.UpdateScheduleOptions{}

	if v, ok := d.GetOk("overflow"); ok {
		opts.Overflow = v.(bool)
	}

	// A schedule layer can never be removed but it can be ended.
	// Here we determine which layer has been removed from the configuration
	// and we mark it as ended. This is to avoid diff issues.

	if d.HasChange("layer") {
		oraw, nraw := d.GetChange("layer")

		osl, err := expandScheduleLayers(oraw.([]interface{}))
		if err != nil {
			return err
		}

		nsl, err := expandScheduleLayers(nraw.([]interface{}))
		if err != nil {
			return err
		}

		// Checks to see if new schedule layers (nsl) include all old schedule layers (osl)
		for _, o := range osl {
			found := false
			for _, n := range nsl {
				// layer is found in both nsl and osl
				if o.ID == n.ID {
					found = true
				}
			}

			// If layer is not found in new schedule layers (nsl) set end value for layer
			if !found {
				end, err := timeToUTC(time.Now().Format(time.RFC3339))
				if err != nil {
					return err
				}
				endStr := end.String()
				o.End = &endStr
				schedule.ScheduleLayers = append(schedule.ScheduleLayers, o)
			}
		}
	}

	log.Printf("[INFO] Updating PagerDuty schedule: %s", d.Id())

	retryErr := resource.Retry(2*time.Minute, func() *resource.RetryError {
		if _, _, err := client.Schedules.Update(d.Id(), schedule, opts); err != nil {
			return resource.RetryableError(err)
		}
		return nil
	})
	if retryErr != nil {
		time.Sleep(2 * time.Second)
		return retryErr
	}

	return nil
}

func resourcePagerDutyScheduleDelete(d *schema.ResourceData, meta interface{}) error {
	client, err := meta.(*Config).Client()
	if err != nil {
		return err
	}
	scheduleId := d.Id()

	log.Printf("[INFO] Starting deletion process of Schedule %s", scheduleId)

	log.Printf("[INFO] Listing Escalation Policies that use schedule : %s", scheduleId)
	// Extracting Escalation Policies that use this Schedule
	epsAssociatedToSchedule, err := extractEPsAssociatedToSchedule(client, scheduleId)
	if err != nil {
		return err
	}

	// An Schedule with open incidents related can't be remove till those
	// incidents have been resolved.
	linksToIncidentsOpen, err := listIncidentsOpenedRelatedToSchedule(client, scheduleId)
	if err != nil {
		return err
	}

	if len(linksToIncidentsOpen) > 0 {
		var urlLinksMessage string
		for _, incident := range linksToIncidentsOpen {
			urlLinksMessage = fmt.Sprintf("%s\n%s", urlLinksMessage, incident)
		}
		return fmt.Errorf("Before Removing Schedule %q You must first resolve the following incidents related with Escalation Policies using this Schedule... %s", scheduleId, urlLinksMessage)
	}

	log.Printf("[INFO] Deleting PagerDuty schedule: %s", scheduleId)
	// Retrying to give other resources (such as escalation policies) to delete
	retryErr := resource.Retry(2*time.Minute, func() *resource.RetryError {
		if _, err := client.Schedules.Delete(scheduleId); err != nil {
			if !isErrCode(err, 400) {
				return resource.RetryableError(err)
			}

			// Handling of specific http 400 errors from API call DELETE /schedules
			if e, ok := err.(*pagerduty.Error); !ok || strings.Compare(fmt.Sprintf("%v", e.Errors), "[Schedule can't be deleted if it's being used by escalation policies]") != 0 {
				return resource.NonRetryableError(err)
			}

			log.Printf("[INFO] Dissociating Escalation Policies that use the Schedule: %s", scheduleId)
			workaroundErr := dissociateScheduleFromEPs(client, scheduleId, epsAssociatedToSchedule)
			if workaroundErr != nil {
				err = fmt.Errorf("%v; %w", err, workaroundErr)
			}
			return resource.RetryableError(err)
		}
		return nil
	})
	if retryErr != nil {
		time.Sleep(2 * time.Second)
		return retryErr
	}

	d.SetId("")

	return nil
}

func expandScheduleLayers(v interface{}) ([]*pagerduty.ScheduleLayer, error) {
	var scheduleLayers []*pagerduty.ScheduleLayer

	for _, sl := range v.([]interface{}) {
		rsl := sl.(map[string]interface{})

		// This is a temporary fix to prevent getting back the wrong rotation_virtual_start time.
		// The background here is that if a user specifies a rotation_virtual_start time to be:
		// "2017-09-01T10:00:00+02:00" the API returns back "2017-09-01T12:00:00+02:00".
		// With this fix in place, we get the correct rotation_virtual_start time, thus
		// eliminating the diff issues we've been seeing in the past.
		// This has been confirmed working by PagerDuty support.
		rvs, err := timeToUTC(rsl["rotation_virtual_start"].(string))
		if err != nil {
			return nil, err
		}

		// The type of layer.*.end is schema.TypeString. If the end is an empty string, it means the layer does not end.
		// A client should send a payload including `"end": null` to unset the end of layer.
		scheduleLayer := &pagerduty.ScheduleLayer{
			ID:                        rsl["id"].(string),
			Name:                      rsl["name"].(string),
			Start:                     rsl["start"].(string),
			End:                       stringTypeToStringPtr(rsl["end"].(string)),
			RotationVirtualStart:      rvs.String(),
			RotationTurnLengthSeconds: rsl["rotation_turn_length_seconds"].(int),
		}

		for _, slu := range rsl["users"].([]interface{}) {
			user := &pagerduty.UserReferenceWrapper{
				User: &pagerduty.UserReference{
					ID:   slu.(string),
					Type: "user",
				},
			}
			scheduleLayer.Users = append(scheduleLayer.Users, user)
		}

		for _, slr := range rsl["restriction"].([]interface{}) {
			rslr := slr.(map[string]interface{})

			restriction := &pagerduty.Restriction{
				Type:            rslr["type"].(string),
				StartTimeOfDay:  rslr["start_time_of_day"].(string),
				StartDayOfWeek:  rslr["start_day_of_week"].(int),
				DurationSeconds: rslr["duration_seconds"].(int),
			}

			scheduleLayer.Restrictions = append(scheduleLayer.Restrictions, restriction)
		}

		scheduleLayers = append(scheduleLayers, scheduleLayer)
	}

	return scheduleLayers, nil
}

func flattenScheduleLayers(v []*pagerduty.ScheduleLayer) ([]map[string]interface{}, error) {
	var scheduleLayers []map[string]interface{}

	for _, sl := range v {
		// A schedule layer can never be removed but it can be ended.
		// Here we check each layer and if it has been ended we don't read it back
		// because it's not relevant anymore.
		endStr := stringPtrToStringType(sl.End)
		if endStr != "" {
			end, err := timeToUTC(endStr)
			if err != nil {
				return nil, err
			}

			if time.Now().UTC().After(end) {
				continue
			}
		}
		scheduleLayer := map[string]interface{}{
			"id":                           sl.ID,
			"name":                         sl.Name,
			"end":                          endStr,
			"start":                        sl.Start,
			"rotation_virtual_start":       sl.RotationVirtualStart,
			"rotation_turn_length_seconds": sl.RotationTurnLengthSeconds,
			"rendered_coverage_percentage": renderRoundedPercentage(sl.RenderedCoveragePercentage),
		}

		var users []string

		for _, slu := range sl.Users {
			users = append(users, slu.User.ID)
		}

		scheduleLayer["users"] = users

		var restrictions []map[string]interface{}

		for _, slr := range sl.Restrictions {
			restriction := map[string]interface{}{
				"duration_seconds":  slr.DurationSeconds,
				"start_time_of_day": slr.StartTimeOfDay,
				"type":              slr.Type,
			}

			if slr.StartDayOfWeek > 0 {
				restriction["start_day_of_week"] = slr.StartDayOfWeek
			}

			restrictions = append(restrictions, restriction)
		}

		scheduleLayer["restriction"] = restrictions

		scheduleLayers = append(scheduleLayers, scheduleLayer)
	}

	// Reverse the final result and return it
	resultReversed := make([]map[string]interface{}, 0, len(scheduleLayers))

	for i := len(scheduleLayers) - 1; i >= 0; i-- {
		resultReversed = append(resultReversed, scheduleLayers[i])
	}

	return resultReversed, nil
}

// the expandShedTeams and flattenSchedTeams are based on the expandTeams and flattenTeams functions in the user
// resource. added these functions here for maintainability
func expandSchedTeams(v interface{}) []*pagerduty.TeamReference {
	var teams []*pagerduty.TeamReference

	for _, t := range v.([]interface{}) {
		team := &pagerduty.TeamReference{
			ID:   t.(string),
			Type: "team_reference",
		}
		teams = append(teams, team)
	}

	return teams
}

func flattenShedTeams(teams []*pagerduty.TeamReference) []string {
	res := make([]string, len(teams))
	for i, t := range teams {
		res[i] = t.ID
	}

	return res
}

func flattenScheFinalSchedule(finalSche *pagerduty.SubSchedule) []map[string]interface{} {
	var res []map[string]interface{}
	elem := make(map[string]interface{})
	elem["name"] = finalSche.Name
	elem["rendered_coverage_percentage"] = renderRoundedPercentage(finalSche.RenderedCoveragePercentage)
	res = append(res, elem)

	return res
}

func listIncidentsOpenedRelatedToSchedule(c *pagerduty.Client, id string) ([]string, error) {
	var s *pagerduty.Schedule
	retryErr := resource.Retry(10*time.Second, func() *resource.RetryError {
		resp, _, err := c.Schedules.Get(id, &pagerduty.GetScheduleOptions{})
		if err != nil {
			time.Sleep(2 * time.Second)
			return resource.RetryableError(err)
		}
		s = resp
		return nil
	})
	if retryErr != nil {
		return nil, retryErr
	}

	teams := []string{}
	for _, t := range s.Teams {
		teams = append(teams, t.ID)
	}

	var linksToIncidents []string
	retryErr = resource.Retry(10*time.Second, func() *resource.RetryError {
		incidents, err := c.Incidents.ListAll(&pagerduty.ListIncidentsOptions{
			DateRange: "all",
			Statuses:  []string{"triggered", "acknowledged"},
			TeamIDs:   teams,
		})
		if err != nil {
			time.Sleep(2 * time.Second)
			return resource.RetryableError(err)
		}
		for _, inc := range incidents {
			linksToIncidents = append(linksToIncidents, inc.HTMLURL)
		}
		return nil
	})

	return linksToIncidents, nil
}

func extractEPsAssociatedToSchedule(c *pagerduty.Client, id string) ([]string, error) {
	var s *pagerduty.Schedule
	retryErr := resource.Retry(10*time.Second, func() *resource.RetryError {
		resp, _, err := c.Schedules.Get(id, &pagerduty.GetScheduleOptions{})
		if err != nil {
			time.Sleep(2 * time.Second)
			return resource.RetryableError(err)
		}
		s = resp
		return nil
	})
	if retryErr != nil {
		return nil, retryErr
	}

	eps := []string{}
	for _, ep := range s.EscalationPolicies {
		eps = append(eps, ep.ID)
	}
	return eps, nil
}

func dissociateScheduleFromEPs(c *pagerduty.Client, scheduleID string, eps []string) error {
	for _, epID := range eps {
		isEPFound := false
		var ep *pagerduty.EscalationPolicy
		errorMessage := fmt.Sprintf("Error while trying to dissociate Schedule %q from Escalation Policy %q", scheduleID, epID)
		retryErr := resource.Retry(10*time.Second, func() *resource.RetryError {
			resp, _, err := c.EscalationPolicies.Get(epID, &pagerduty.GetEscalationPolicyOptions{})
			if err != nil {
				if isErrCode(err, 404) {
					return nil
				}
				return resource.RetryableError(err)
			}
			ep = resp
			isEPFound = true
			return nil
		})
		if retryErr != nil {
			return fmt.Errorf("%w; %s", retryErr, errorMessage)
		}

		if !isEPFound {
			continue
		}
		err := removeScheduleFromEP(c, scheduleID, ep)
		if err != nil {
			return fmt.Errorf("%w; %s", err, errorMessage)
		}
	}
	return nil
}

func removeScheduleFromEP(c *pagerduty.Client, scheduleID string, ep *pagerduty.EscalationPolicy) error {
	needsToUpdate := false
	epr := ep.EscalationRules
	for ri, r := range epr {
		for index, target := range r.Targets {
			isScheduleConfiguredInEscalationRule := target.Type == "schedule_reference" && target.ID == scheduleID
			if !isScheduleConfiguredInEscalationRule {
				continue
			}

			if isScheduleConfiguredInEscalationRule {
				if len(r.Targets) > 1 {
					// Removing Schedule as a configured Target from the Escalation Rules
					// slice.
					r.Targets = append(r.Targets[:index], r.Targets[index+1:]...)
				} else {
					// Removing Escalation Rules that will end up having no target configured.
					epr = append(epr[:ri], epr[ri+1:]...)
				}
				needsToUpdate = true
			}
		}
	}
	if !needsToUpdate {
		return nil
	}
	ep.EscalationRules = epr

	retryErr := resource.Retry(10*time.Second, func() *resource.RetryError {
		_, _, err := c.EscalationPolicies.Update(ep.ID, ep)
		if err != nil && !isErrCode(err, 404) {
			return resource.RetryableError(err)
		}
		return nil
	})
	if retryErr != nil {
		return retryErr
	}

	return nil
}
