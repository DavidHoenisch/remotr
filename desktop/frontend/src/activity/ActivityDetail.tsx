import { FileKey2, ScrollText } from "lucide-react";

import type { ActivityEventView } from "./ActivityPage";
import "./ActivityPage.css";

export function ActivityDetail({ event }: { event: ActivityEventView }) {
  return (
    <div className="activity-detail">
      <div className="activity-detail-callout">
        <ScrollText aria-hidden="true" size={15} strokeWidth={1.8} />
        <div>
          <strong>Server audit evidence</strong>
          <span>Values are rendered as inert structured text.</span>
        </div>
      </div>

      <dl className="activity-detail-summary">
        <div>
          <dt>Event time</dt>
          <dd>{event.occurredAt}</dd>
        </div>
        <div>
          <dt>Actor</dt>
          <dd>{event.actor}</dd>
        </div>
        <div>
          <dt>Action</dt>
          <dd>{event.action}</dd>
        </div>
        <div>
          <dt>Resource</dt>
          <dd>
            {event.resourceType} / {event.resourceId}
          </dd>
        </div>
        <div>
          <dt>Status</dt>
          <dd>{event.status}</dd>
        </div>
        <div>
          <dt>Request ID</dt>
          <dd>{event.requestId || "Not reported"}</dd>
        </div>
      </dl>

      <section
        aria-label="Structured audit details"
        className="activity-detail-fields"
      >
        <header>
          <FileKey2 aria-hidden="true" size={15} strokeWidth={1.8} />
          <h3>Structured audit details</h3>
          <span>{event.details.length} fields</span>
        </header>
        {event.details.length > 0 ? (
          <dl>
            {event.details.map((detail, index) => (
              <div key={`${detail.key}-${index}`}>
                <dt>{detail.key}</dt>
                <dd>{detail.value}</dd>
              </div>
            ))}
          </dl>
        ) : (
          <p>No bounded detail fields were reported.</p>
        )}
      </section>
    </div>
  );
}
