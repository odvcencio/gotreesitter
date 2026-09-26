'* Copyright (c) 2017 Roku, Inc. All rights reserved.

'
' File: SampleVideo.brs
'
'* video.manifestData field is roAssociativeArray which currently carries two elements:
'*    "mpd"     - roAssociativeArray of string values
'*    "periods" - roArray of roAssociativeArrays of string values
'*
'* Examples of accessing manifestData:
'*
'*   1) Get a known attribute:
'*      video.manifestData.mpd.minimumUpdatePeriod
'*
'*   2) Get a known attribute which has a semicolon in the name:
'*      video.manifestData.mpd["xmlns:ns1"]
'*
'*   3) Get a known attribute from existing period:
'*      video.manifestData.period[0].id
'*
'*   4) Get number of available periods
'*      video.manifestData.periods.Count()
'*
'*   5) Iterate through all available MPD attributes
'*      for each item in video.manifestData.mpd.Items()
'*        print item.key, "=", item.value
'*      end for

function init()
    print m.top.subType() + ".init()"

    video = m.top
    content = createObject("RoSGNode", "ContentNode")
    content.setFields({streamFormat:"dash", Live:true})

    testCase = 2

    if      testCase = 1
          content.setFields({URL: "http://vm2.dashif.org/livesim-dev/segtimeline_1/testpic_2s/Manifest.mpd" })

    else if testCase = 2 '#--- SegmentTemplate with manifest updates every 30s
          content.setFields({URL: "http://vm2.dashif.org/livesim/mup_30/testpic_2s/Manifest.mpd" })

    end if

    video.observeField("manifestData", "manifestDataChanged")
    video.content = content
    video.control = "play"
end function

function manifestDataChanged()
    print "---"
    print "video.manifestData field changed"
    print "---"
    video = m.top
    md = video.manifestData

    print "<MPD"
    for each item in md.mpd.Items()
        print "   "; item.key; "="""; item.value; """"
    end for
    print ">"

    for each period in md.periods
        print "<Period"
        for each item in period.Items()
           print "   "; item.key; "="""; item.value; """"
        end for
        print ">"
    end for
end function
