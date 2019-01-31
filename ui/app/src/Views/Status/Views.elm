module Views.Status.Views exposing (view)

import Data.AlertmanagerStatus exposing (AlertmanagerStatus)
import Data.ClusterStatus exposing (ClusterStatus, Status(..))
import Data.PeerStatus exposing (PeerStatus)
import Data.VersionInfo exposing (VersionInfo)
import Html exposing (..)
import Html.Attributes exposing (class, classList, style)
import Status.Api exposing (clusterStatusToString)
import Status.Types exposing (StatusResponse, VersionInfo)
import Types exposing (Msg(..))
import Utils.Date exposing (timeToString)
import Utils.Types exposing (ApiData(..))
import Utils.Views
import Views.Status.Types exposing (StatusModel)


view : StatusModel -> Html Types.Msg
view { statusInfo } =
    case statusInfo of
        Success info ->
            viewStatusInfo info

        Initial ->
            Utils.Views.loading

        Loading ->
            Utils.Views.loading

        Failure msg ->
            Utils.Views.error msg


viewStatusInfo : AlertmanagerStatus -> Html Types.Msg
viewStatusInfo status =
    div []
        [ h1 [] [ text "Status" ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "Uptime:" ]
            , div [ class "col-sm-10" ] [ text <| timeToString status.uptime ]
            ]
        , viewClusterStatus status.cluster
        , viewVersionInformation status.versionInfo
        , viewConfig status.config.original
        ]


viewConfig : String -> Html Types.Msg
viewConfig config =
    div []
        [ h2 [] [ text "Config" ]
        , pre [ class "p-4", style "background" "#f7f7f9", style "font-family" "monospace" ]
            [ code []
                [ text config
                ]
            ]
        ]


viewClusterStatus : ClusterStatus -> Html Types.Msg
viewClusterStatus { name, status, peers } =
    span []
        [ h2 [] [ text "Cluster Status" ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "Name:" ]
            , div [ class "col-sm-10" ] [ text name ]
            ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "Status:" ]
            , div [ class "col-sm-10" ]
                [ span
                    [ classList
                        [ ( "badge", True )
                        , case status of
                            Ready ->
                                ( "badge-success", True )

                            Settling ->
                                ( "badge-warning", True )

                            Disabled ->
                                ( "badge-danger", True )
                        ]
                    ]
                    [ text <| clusterStatusToString status ]
                ]
            ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "Peers:" ]
            , ul [ class "col-sm-10" ] <|
                List.map viewClusterPeer peers
            ]
        ]


viewClusterPeer : PeerStatus -> Html Types.Msg
viewClusterPeer peer =
    li []
        [ div [ class "" ]
            [ b [ class "" ] [ text "Name: " ]
            , text peer.name
            ]
        , div [ class "" ]
            [ b [ class "" ] [ text "Address: " ]
            , text peer.address
            ]
        ]


viewVersionInformation : VersionInfo -> Html Types.Msg
viewVersionInformation versionInfo =
    span []
        [ h2 [] [ text "Version Information" ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "Branch:" ], div [ class "col-sm-10" ] [ text versionInfo.branch ] ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "BuildDate:" ], div [ class "col-sm-10" ] [ text versionInfo.buildDate ] ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "BuildUser:" ], div [ class "col-sm-10" ] [ text versionInfo.buildUser ] ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "GoVersion:" ], div [ class "col-sm-10" ] [ text versionInfo.goVersion ] ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "Revision:" ], div [ class "col-sm-10" ] [ text versionInfo.revision ] ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "Version:" ], div [ class "col-sm-10" ] [ text versionInfo.version ] ]
        ]
