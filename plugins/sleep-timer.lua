-- sleep-timer.lua — Stop playback after a set time, fading out first.
--
-- Press W to cycle: off -> 15m -> 30m -> 45m -> 60m -> off.
-- Also scriptable:  grbfy plugins call sleep-timer set 30
--                   grbfy plugins call sleep-timer cancel
--                   grbfy plugins call sleep-timer status

local p = plugin.register({
    name        = "sleep-timer",
    type        = "hook",
    version     = "1.0.0",
    description = "Fade out and stop playback after a set time",
    permissions = {"control", "keymap"},
})

-- Fade length is configurable:
--
--   [plugins.sleep-timer]
--   fade_seconds = 20
local FADE_SECS = tonumber(p:config("fade_seconds")) or 20
if FADE_SECS <= 0 then FADE_SECS = 20 end

local FADE_STEPS = 20     -- volume steps spread across the fade
local MIN_VOLUME = -30    -- the API floor; below this it is inaudible anyway

local choices = {0, 15, 30, 45, 60}   -- minutes; 0 means off
local choice_idx = 1

local expire_timer = nil
local fade_timer = nil
local warn_timer = nil
local restore_volume = nil            -- volume to put back after we stop
local ends_at = nil                   -- os.time() when playback stops

local function clear_timers()
    for _, id in pairs({expire_timer, fade_timer, warn_timer}) do
        if id then grbfy.timer.cancel(id) end
    end
    expire_timer, fade_timer, warn_timer = nil, nil, nil
end

-- restore_after_stop puts the volume back once playback has stopped, so the
-- next session does not start silently. Without this the fade would be a
-- permanent volume change.
local function restore_after_stop()
    if restore_volume then
        grbfy.player.set_volume(restore_volume)
        restore_volume = nil
    end
end

local function cancel(reason)
    clear_timers()
    restore_after_stop()
    ends_at = nil
    choice_idx = 1
    if reason then grbfy.message(reason) end
end

local function begin_fade()
    restore_volume = grbfy.player.volume()
    local from = restore_volume
    local step = (from - MIN_VOLUME) / FADE_STEPS
    local n = 0

    fade_timer = grbfy.timer.every(FADE_SECS / FADE_STEPS, function()
        n = n + 1
        if n >= FADE_STEPS then
            grbfy.timer.cancel(fade_timer)
            fade_timer = nil
            grbfy.player.stop()
            restore_after_stop()
            ends_at = nil
            choice_idx = 1
            grbfy.notify("Sleep timer", "Playback stopped")
            grbfy.log.info("sleep timer elapsed; playback stopped")
            return
        end
        grbfy.player.set_volume(from - step * n)
    end)
end

local function arm(minutes)
    clear_timers()
    restore_after_stop()

    if minutes <= 0 then
        ends_at = nil
        grbfy.message("Sleep timer off")
        return
    end

    local total = minutes * 60
    ends_at = os.time() + total

    -- Fade during the final stretch so the timer stops AT the requested time
    -- rather than starting to fade then.
    local fade_at = total - FADE_SECS
    if fade_at < 1 then fade_at = 1 end
    expire_timer = grbfy.timer.after(fade_at, begin_fade)

    if total > 70 then
        warn_timer = grbfy.timer.after(total - 60, function()
            grbfy.notify("Sleep timer", "1 minute left")
        end)
    end

    grbfy.message("Sleep timer: " .. minutes .. "m")
    grbfy.log.info("sleep timer armed for " .. minutes .. "m")
end

local function status_text()
    if not ends_at then return "Sleep timer is off" end
    local left = ends_at - os.time()
    if left < 0 then left = 0 end
    return string.format("Sleep timer: %dm %ds left", math.floor(left / 60), left % 60)
end

local bound, why = p:bind("W", "Sleep timer", function()
    choice_idx = choice_idx % #choices + 1
    arm(choices[choice_idx])
end)
if not bound then
    grbfy.log.warn("could not bind W: " .. tostring(why) .. " (use `grbfy plugins call sleep-timer set <minutes>`)")
end

p:command("set", function(args)
    local minutes = tonumber(args and args[1])
    if not minutes then return "usage: set <minutes>" end
    arm(minutes)
    return status_text()
end)

p:command("cancel", function()
    cancel(nil)
    return "Sleep timer cancelled"
end)

p:command("status", function()
    return status_text()
end)

-- Leaving the volume faded down would outlive the session that caused it.
p:on("app.quit", function()
    clear_timers()
    restore_after_stop()
end)
