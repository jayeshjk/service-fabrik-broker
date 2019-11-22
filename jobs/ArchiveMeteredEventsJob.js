'use strict';

const _ = require('lodash');
const CONST = require('../common/constants');
const config = require('../common/config');
const apiServerClient = require('../data-access-layer/eventmesh').apiServerClient;
const meteringArchiveStore = require('../data-access-layer/iaas').meteringArchiveStore;
const BaseJob = require('./BaseJob');
const logger = require('../common/logger');
const utils = require('../common/utils');

class ArchiveMeteredEventsJob extends BaseJob {

  static async run(job, done) {
    try {
      logger.info(`-> Starting ArchiveMeteredEventsJob -  name: ${job.attrs.data[CONST.JOB_NAME_ATTRIB]} - with options: ${JSON.stringify(job.attrs.data)} `);
      const events = await this.getMeteredEvents();
      logger.info(`Total number of metered events obtained from ApiServer: ${events.length}`);
      if(events.length === 0) {
        return this.runSucceeded({}, job, done);
      }
      const meteringFileTimeStamp = new Date();
      const successfullyPatchedEvents = await this.patchToMeteringStore(events, meteringFileTimeStamp.toISOString(), job.attrs.data.sleepDuration, job.attrs.data.deleteAttempts);
      logger.info(`No of processed metered events: ${successfullyPatchedEvents}`);
      return this.runSucceeded({}, job, done);
    } catch(err) {
      return this.runFailed(err, {}, job, done);
    }
  }

  static async getMeteredEvents() {
    let selector = `state in (${CONST.METER_STATE.METERED})`;
    const options = {
      resourceGroup: CONST.APISERVER.RESOURCE_GROUPS.INSTANCE,
      resourceType: CONST.APISERVER.RESOURCE_TYPES.SFEVENT,
      query: {
        labelSelector: selector
      }
    };
    return apiServerClient.getResources(options);
  }

  static async patchToMeteringStore(events, timeStamp, sleepDuration, attempts) {
    try {
      await meteringArchiveStore.putArchiveFile(timeStamp);
      const noEventsToPatch = Math.min(_.get(config, 'system_jobs.archive_metered_events.job_data.events_to_patch', CONST.ARCHIVE_METERED_EVENTS_RUN_THRESHOLD), events.length);
      const eventsToPatch = _.slice(events, 0, noEventsToPatch);
      for(let i = 0; i < eventsToPatch.length; i++) {
        await this.processEvent(eventsToPatch[i], timeStamp, attempts || 4);
        await utils.sleep(sleepDuration || 1000);
      }
      return noEventsToPatch;
    } catch(err) {
      logger.error('Error while archiving events in the MeteringStore: ', err);
      throw err;
    }
  }

  static async processEvent(event, timeStamp, attempts) {
    logger.info(`Processing event: ${event.metadata.name}`);
    await meteringArchiveStore.patchEventToArchiveFile(event, timeStamp);
    return utils.retry(tries => {
      logger.debug(`Trying to delete ${event.metadata.name}. Total retries yet: ${tries}`);
      return apiServerClient.deleteResource({
        resourceGroup: CONST.APISERVER.RESOURCE_GROUPS.INSTANCE,
        resourceType: CONST.APISERVER.RESOURCE_TYPES.SFEVENT,
        resourceId: `${event.metadata.name}`
      });
    }, {
      maxAttempts: attempts || 4,
      minDelay: 1000
    });
  }
}

module.exports = ArchiveMeteredEventsJob;
