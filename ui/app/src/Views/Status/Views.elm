module Views.Status.Views exposing (view)

import Data.AlertmanagerStatus exposing (AlertmanagerStatus)
import Data.ClusterStatus exposing (ClusterStatus, Status(..))
import Data.PeerStatus exposing (PeerStatus)
import Data.VersionInfo exposing (VersionInfo)
import Html exposing (Html, b, code, div, h1, h2, li, pre, span, text, ul)
import Html.Attributes exposing (class, classList, style)
import Status.Api exposing (clusterStatusToString)
import Status.Types exposing (VersionInfo)
import Types
import Utils.Date exposing (timeToString)
import Utils.Types exposing (ApiData(..))
import Utils.Views
import Views.Status.Types exposing (StatusModel)


view : StatusModel -> Html Types.Msg
view { statusInfo } =
    Utils.Views.apiData viewStatusInfo statusInfo


viewStatusInfo : AlertmanagerStatus -> Html Types.Msg
viewStatusInfo status =
    div []
        [ h1 [] [ text "Status" ]
        , div [ class "row mb-3" ]
            [ b [ class "col-sm-2 form-label" ] [ text "Uptime:" ]
            , div [ class "col-sm-10" ] [ text <| timeToString status.uptime ]
            ]
        , viewClusterStatus status.cluster
        , viewVersionInformation status.versionInfo
        , viewConfig status.config.original
        ]


viewConfig : String -> Html Types.Msg
viewConfig config =
    div []
        [ h2 [ class "mb-3" ] [ text "Config" ]
        , pre [ class "p-4 bg-body-secondary rounded", style "font-family" "monospace" ]
            [ code [] [ text config ] ]
        ]


viewClusterStatus : ClusterStatus -> Html Types.Msg
viewClusterStatus { name, status, peers } =
    div []
        [ h2 [] [ text "Cluster Status" ]
        , case name of
            Just n ->
                div [ class "row mb-3" ]
                    [ b [ class "col-sm-2 form-label" ] [ text "Name:" ]
                    , div [ class "col-sm-10" ] [ text n ]
                    ]

            Nothing ->
                text ""
        , div [ class "row mb-3" ]
            [ b [ class "col-sm-2 form-label" ] [ text "Status:" ]
            , div [ class "col-sm-10" ]
                [ span
                    [ classList
                        [ ( "badge", True )
                        , case status of
                            Ready ->
                                ( "bg-success", True )

                            Settling ->
                                ( "bg-warning", True )

                            Disabled ->
                                ( "bg-danger", True )
                        ]
                    ]
                    [ text <| clusterStatusToString status ]
                ]
            ]
        , case peers of
            Just p ->
                div [ class "row mb-3" ]
                    [ b [ class "col-sm-2 form-label" ] [ text "Peers:" ]
                    , ul [ class "col-sm-10 list-group" ]
                        (List.map viewClusterPeer p)
                    ]

            Nothing ->
                text ""
        ]


viewClusterPeer : PeerStatus -> Html Types.Msg
viewClusterPeer peer =
    li []
        [ div [ class "mb-2" ]
            [ b [ class "me-2" ] [ text "Name:" ]
            , text peer.name
            ]
        , div []
            [ b [ class "me-2" ] [ text "Address:" ]
            , text peer.address
            ]
        ]


viewVersionInformation : VersionInfo -> Html Types.Msg
viewVersionInformation versionInfo =
    div []
        [ h2 [ class "mb-3" ] [ text "Version Information" ]
        , div [ class "row mb-2" ]
            [ b [ class "col-sm-2 form-label" ] [ text "Branch:" ]
            , div [ class "col-sm-10" ] [ text versionInfo.branch ]
            ]
        , div [ class "row mb-2" ]
            [ b [ class "col-sm-2 form-label" ] [ text "BuildDate:" ]
            , div [ class "col-sm-10" ] [ text versionInfo.buildDate ]
            ]
        , div [ class "row mb-2" ]
            [ b [ class "col-sm-2 form-label" ] [ text "BuildUser:" ]
            , div [ class "col-sm-10" ] [ text versionInfo.buildUser ]
            ]
        , div [ class "row mb-2" ]
            [ b [ class "col-sm-2 form-label" ] [ text "GoVersion:" ]
            , div [ class "col-sm-10" ] [ text versionInfo.goVersion ]
            ]
        , div [ class "row mb-2" ]
            [ b [ class "col-sm-2 form-label" ] [ text "Revision:" ]
            , div [ class "col-sm-10" ] [ text versionInfo.revision ]
            ]
        , div [ class "row mb-2" ]
            [ b [ class "col-sm-2 form-label" ] [ text "Version:" ]
            , div [ class "col-sm-10" ] [ text versionInfo.version ]
            ]
        ]
