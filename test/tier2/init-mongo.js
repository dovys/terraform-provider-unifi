// The pinned linuxserver image builds a Mongo connection string that must carry
// a username, so it needs these users to exist (it refuses to start with "No
// username is provided in the connection string" otherwise). Mongo itself runs
// without auth enforcement here — this is a throwaway, network-isolated test
// controller — so the credentials only have to be present, not secret.
db.getSiblingDB("unifi").createUser({
  user: "unifi",
  pwd: "unifi",
  roles: [{ role: "dbOwner", db: "unifi" }],
});
db.getSiblingDB("unifi_stat").createUser({
  user: "unifi",
  pwd: "unifi",
  roles: [{ role: "dbOwner", db: "unifi_stat" }],
});
